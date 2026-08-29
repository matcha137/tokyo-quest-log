// Package worldsim はワールドマップ上の探索ロジックを扱う。
//
// 描画には依存しない。移動・通行判定・遭遇・ランドマークへの到着といった
// 判断はすべてここで完結させ、Ebitengine側は入力と描画だけを受け持つ。
package worldsim

import (
	"math"
	"math/rand"

	"tokyo-quest-log/internal/geomesh"
	"tokyo-quest-log/internal/landmark"
	"tokyo-quest-log/internal/worldgrid"
)

// Vehicle は移動手段。進入できる地形が変わる。
type Vehicle int

const (
	OnFoot Vehicle = iota // 徒歩
	ByShip                // 船。海に出られる
)

func (v Vehicle) String() string {
	if v == ByShip {
		return "船"
	}
	return "徒歩"
}

// Vec はマス単位の連続座標。X は西から東、Y は北から南へ増える。
type Vec struct {
	X float64
	Y float64
}

// encounterRates は地形ごとの遭遇しやすさ。1マス進むごとに加算される。
// 進入できない地形は0。険しい地形ほど高くし、高山は最も遭遇しやすい。
// 町の中は別途、無条件で安全にする。
var encounterRates = [worldgrid.TerrainCount]float64{
	worldgrid.Sea:          0.5,
	worldgrid.Lowland:      0.7,
	worldgrid.Plain:        0.9,
	worldgrid.Plateau:      1.1,
	worldgrid.Hill:         1.5,
	worldgrid.Mountain:     2.0,
	worldgrid.HighMountain: 2.6,
	worldgrid.Water:        0,
	worldgrid.OutOfArea:    0,
}

// World はワールドマップの探索状態。
type World struct {
	Grid      *worldgrid.Grid
	Placed    []landmark.Placed
	Landmarks landmark.Index

	Player  Vec
	Vehicle Vehicle
	Steps   float64 // 累積移動距離（マス）

	rng             *rand.Rand
	encounterBudget float64
	nextEncounter   float64
	lastCell        [2]int
}

// New は探索状態を作る。プレイヤーは最初のランドマーク、
// ランドマークが無ければ最初に見つかった通行可能なマスに立つ。
func New(g *worldgrid.Grid, placed []landmark.Placed, seed int64) *World {
	w := &World{
		Grid:      g,
		Placed:    placed,
		Landmarks: landmark.NewIndex(placed),
		Vehicle:   OnFoot,
		rng:       rand.New(rand.NewSource(seed)),
	}
	w.armEncounter()
	if len(placed) > 0 {
		w.placeAt(placed[0].Row, placed[0].Col)
	} else {
		w.spawnAnywhere()
	}
	return w
}

// SpawnAt は指定IDのランドマークへプレイヤーを移す。
func (w *World) SpawnAt(id string) bool {
	for _, p := range w.Placed {
		if p.ID == id {
			w.placeAt(p.Row, p.Col)
			return true
		}
	}
	return false
}

func (w *World) placeAt(row, col int) {
	w.Player = Vec{X: float64(col) + 0.5, Y: float64(row) + 0.5}
	w.lastCell = [2]int{row, col}
}

func (w *World) spawnAnywhere() {
	for row := 0; row < w.Grid.Rows; row++ {
		for col := 0; col < w.Grid.Cols; col++ {
			if w.Grid.At(row, col).Walkable() {
				w.placeAt(row, col)
				return
			}
		}
	}
}

// Cell はプレイヤーが立っているマスを返す。
func (w *World) Cell() (row, col int) {
	return int(w.Player.Y), int(w.Player.X)
}

// Tile はプレイヤーが立っている地形を返す。
func (w *World) Tile() worldgrid.Terrain {
	row, col := w.Cell()
	return w.Grid.At(row, col)
}

// LatLon はプレイヤーの現在地を緯度経度で返す。
func (w *World) LatLon() geomesh.LatLon {
	return geomesh.LatLon{
		Lat: w.Grid.OriginLat + (float64(w.Grid.Rows)-w.Player.Y)*w.Grid.LatSpan,
		Lon: w.Grid.OriginLon + w.Player.X*w.Grid.LonSpan,
	}
}

// LandmarkHere は現在のマスにあるランドマークを返す。無ければ nil。
func (w *World) LandmarkHere() *landmark.Placed {
	row, col := w.Cell()
	return w.Landmarks.At(row, col)
}

// CanEnter は指定マスへ進入できるかを返す。
func (w *World) CanEnter(row, col int) bool {
	if row < 0 || row >= w.Grid.Rows || col < 0 || col >= w.Grid.Cols {
		return false
	}
	t := w.Grid.At(row, col)
	if w.Vehicle == ByShip {
		// 船は海を進み、岸に着けるよう陸地にも上がれる。
		return t == worldgrid.Sea || t.Walkable()
	}
	return t.Walkable()
}

// StepResult は1回の移動で起きたことをまとめる。
type StepResult struct {
	Moved     bool
	Blocked   bool             // 壁に阻まれて1軸でも動けなかった
	Encounter bool             // 遭遇が発生した
	Arrived   *landmark.Placed // 新しくランドマークのマスへ入った
	Left      *landmark.Placed // ランドマークのマスから出た
}

// Step は方向 (dx, dy) へ distance マス進もうとする。
// 軸ごとに判定するため、壁に斜めに当たったときは沿って滑る。
func (w *World) Step(dx, dy, distance float64) StepResult {
	var result StepResult
	if dx == 0 && dy == 0 || distance <= 0 {
		return result
	}
	length := math.Hypot(dx, dy)
	moveX := dx / length * distance
	moveY := dy / length * distance

	before := w.Player
	beforeCell := w.lastCell

	if moved := w.tryAxis(moveX, 0); !moved && moveX != 0 {
		result.Blocked = true
	}
	if moved := w.tryAxis(0, moveY); !moved && moveY != 0 {
		result.Blocked = true
	}

	travelled := math.Hypot(w.Player.X-before.X, w.Player.Y-before.Y)
	if travelled == 0 {
		return result
	}
	result.Moved = true
	w.Steps += travelled

	row, col := w.Cell()
	if [2]int{row, col} != beforeCell {
		w.lastCell = [2]int{row, col}
		result.Arrived = w.Landmarks.At(row, col)
		result.Left = w.Landmarks.At(beforeCell[0], beforeCell[1])
	}
	result.Encounter = w.advanceEncounter(travelled)
	return result
}

// tryAxis は1軸だけ動かす。進入先が通れなければ動かさない。
func (w *World) tryAxis(dx, dy float64) bool {
	next := Vec{X: w.Player.X + dx, Y: w.Player.Y + dy}
	if !w.CanEnter(int(next.Y), int(next.X)) {
		return false
	}
	w.Player = next
	return true
}

// advanceEncounter は移動量に応じて遭遇の判定を進める。
// ランドマークのマスの上では発生しない。町の中は安全という扱い。
func (w *World) advanceEncounter(distance float64) bool {
	if w.LandmarkHere() != nil {
		return false
	}
	rate := encounterRates[w.Tile()]
	if rate <= 0 {
		return false
	}
	w.encounterBudget += rate * distance
	if w.encounterBudget < w.nextEncounter {
		return false
	}
	w.encounterBudget = 0
	w.armEncounter()
	return true
}

func (w *World) armEncounter() {
	// 一定間隔だと読まれるため、ある程度ばらつかせる。
	w.nextEncounter = 8 + w.rng.Float64()*14
}

// Camera は表示範囲の左上をマス単位で返す。
// プレイヤーを中央に置きつつ、地図の外側を映さないよう端で止める。
func (w *World) Camera(viewCols, viewRows float64) Vec {
	cam := Vec{X: w.Player.X - viewCols/2, Y: w.Player.Y - viewRows/2}
	cam.X = clampCamera(cam.X, viewCols, float64(w.Grid.Cols))
	cam.Y = clampCamera(cam.Y, viewRows, float64(w.Grid.Rows))
	return cam
}

func clampCamera(value, view, total float64) float64 {
	if view >= total {
		// 地図の方が狭いときは中央に寄せる。
		return (total - view) / 2
	}
	return math.Max(0, math.Min(value, total-view))
}

// NearestLandmark はプレイヤーから radius マス以内で最も近いランドマークを返す。
func (w *World) NearestLandmark(radius float64) *landmark.Placed {
	var best *landmark.Placed
	bestDist := radius
	for i := range w.Placed {
		p := &w.Placed[i]
		d := math.Hypot(float64(p.Col)+0.5-w.Player.X, float64(p.Row)+0.5-w.Player.Y)
		if d <= bestDist {
			best, bestDist = p, d
		}
	}
	return best
}

// DistanceMeters は累積移動距離を実世界の概算メートルに換算する。
func (w *World) DistanceMeters() float64 {
	const degreeMeters = 111_320.0
	latMeters := w.Grid.LatSpan * degreeMeters
	lonMeters := w.Grid.LonSpan * degreeMeters * math.Cos(w.Grid.OriginLat*math.Pi/180)
	return w.Steps * (latMeters + lonMeters) / 2
}

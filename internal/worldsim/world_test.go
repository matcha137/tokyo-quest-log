package worldsim

import (
	"math"
	"testing"

	"tokyo-quest-log/internal/landmark"
	"tokyo-quest-log/internal/worldgrid"
)

// testWorld は10×10の平地に、海と高山の壁を置いたグリッドを作る。
func testWorld(t *testing.T) *World {
	t.Helper()
	g := buildTestGrid()
	placed := []landmark.Placed{
		{Landmark: landmark.Landmark{ID: "home", Name: "拠点", Kind: landmark.KindCity}, Row: 5, Col: 5},
	}
	return New(g, placed, 1)
}

func buildTestGrid() *worldgrid.Grid {
	g := &worldgrid.Grid{
		Level: 5, OriginLat: 35.0, OriginLon: 139.0,
		LatSpan: 0.01, LonSpan: 0.01, Cols: 10, Rows: 10,
		Tiles: make([]worldgrid.Terrain, 100),
	}
	for i := range g.Tiles {
		g.Tiles[i] = worldgrid.Plain
	}
	for row := 0; row < 10; row++ {
		g.Set(row, 0, worldgrid.Sea)       // 西の海
		g.Set(row, 9, worldgrid.OutOfArea) // 東は都域外
	}
	return g
}

func TestNewSpawnsOnLandmark(t *testing.T) {
	w := testWorld(t)
	row, col := w.Cell()
	if row != 5 || col != 5 {
		t.Errorf("初期位置 = (%d,%d), want (5,5)", row, col)
	}
	if w.LandmarkHere() == nil {
		t.Error("ランドマークの上に立っていない")
	}
}

func TestSpawnAt(t *testing.T) {
	w := testWorld(t)
	if !w.SpawnAt("home") {
		t.Error("既存のIDで移動できない")
	}
	if w.SpawnAt("存在しない") {
		t.Error("存在しないIDで true が返った")
	}
}

// 徒歩では海にも高山にも入れないこと。
func TestOnFootCannotEnterSeaOrOutOfArea(t *testing.T) {
	w := testWorld(t)
	if w.CanEnter(5, 0) {
		t.Error("徒歩で海に入れてしまう")
	}
	if w.CanEnter(5, 9) {
		t.Error("徒歩で都域外に出られてしまう")
	}
	if !w.CanEnter(5, 4) {
		t.Error("平地に入れない")
	}
}

// 船は海に出られるが、都域外へは出られないこと。
func TestShipEntersSeaOnly(t *testing.T) {
	w := testWorld(t)
	w.Vehicle = ByShip
	if !w.CanEnter(5, 0) {
		t.Error("船で海に出られない")
	}
	if w.CanEnter(5, 9) {
		t.Error("船で都域外に出られてしまう")
	}
	if !w.CanEnter(5, 4) {
		t.Error("船から陸へ上がれない")
	}
}

func TestStepBlockedByTerrain(t *testing.T) {
	w := testWorld(t)
	w.Player = Vec{X: 1.5, Y: 5.5} // 海のすぐ東
	result := w.Step(-1, 0, 1.0)
	if result.Moved {
		t.Error("海へ進めてしまった")
	}
	if !result.Blocked {
		t.Error("阻まれたことが報告されない")
	}
	if w.Player.X != 1.5 {
		t.Errorf("位置が動いた: %v", w.Player.X)
	}
}

// 斜めに壁へ当たったとき、通れる軸だけ動いて壁に沿って滑ること。
func TestStepSlidesAlongWall(t *testing.T) {
	w := testWorld(t)
	w.Player = Vec{X: 1.5, Y: 5.5}
	result := w.Step(-1, 1, 1.0) // 南西へ。西は海
	if !result.Moved {
		t.Fatal("南へも動けていない")
	}
	if w.Player.X != 1.5 {
		t.Errorf("海側へ動いた: X = %v", w.Player.X)
	}
	if w.Player.Y <= 5.5 {
		t.Errorf("南へ動いていない: Y = %v", w.Player.Y)
	}
}

func TestStepAccumulatesDistance(t *testing.T) {
	w := testWorld(t)
	w.Player = Vec{X: 4.5, Y: 4.5}
	w.Step(1, 0, 0.5)
	w.Step(1, 0, 0.5)
	if math.Abs(w.Steps-1.0) > 1e-9 {
		t.Errorf("累積距離 = %v, want 1.0", w.Steps)
	}
	if w.DistanceMeters() <= 0 {
		t.Error("実距離への換算が0以下")
	}
}

// ランドマークのマスへ入った/出たことが報告されること。
func TestStepReportsLandmarkTransition(t *testing.T) {
	w := testWorld(t)
	w.Player = Vec{X: 4.5, Y: 5.5} // 拠点(5,5)の西隣
	w.lastCell = [2]int{5, 4}

	result := w.Step(1, 0, 1.0)
	if result.Arrived == nil || result.Arrived.ID != "home" {
		t.Fatalf("到着が報告されない: %+v", result.Arrived)
	}

	result = w.Step(1, 0, 1.0)
	if result.Left == nil || result.Left.ID != "home" {
		t.Errorf("退出が報告されない: %+v", result.Left)
	}
}

// 町の中では遭遇しないこと。
func TestNoEncounterOnLandmark(t *testing.T) {
	w := testWorld(t)
	w.nextEncounter = 0.001 // すぐ発生する状態にする
	w.Player = Vec{X: 5.2, Y: 5.5}
	result := w.Step(1, 0, 0.2) // 拠点のマス内で動く
	if result.Encounter {
		t.Error("町の中で遭遇した")
	}
}

// 平地を歩き続ければいずれ遭遇し、繰り返し発生すること。
func TestEncounterEventuallyHappens(t *testing.T) {
	w := testWorld(t)
	w.Player = Vec{X: 2.5, Y: 2.5}
	encounters := 0
	for i := 0; i < 2000; i++ {
		dir := 1.0
		if i%2 == 1 {
			dir = -1
		}
		if w.Step(dir, 0, 0.5).Encounter {
			encounters++
		}
	}
	if encounters < 2 {
		t.Errorf("遭遇回数 = %d, 複数回を期待", encounters)
	}
}

// 通れない地形の上では遭遇判定が進まないこと。
func TestNoEncounterOnZeroRateTerrain(t *testing.T) {
	w := testWorld(t)
	w.Grid.Set(3, 3, worldgrid.Water)
	w.Player = Vec{X: 3.5, Y: 3.5}
	w.nextEncounter = 0.001
	if w.advanceEncounter(10) {
		t.Error("遭遇率0の地形で遭遇した")
	}
}

func TestCameraClampsToMap(t *testing.T) {
	w := testWorld(t)
	w.Player = Vec{X: 0.5, Y: 0.5}
	cam := w.Camera(4, 4)
	if cam.X < 0 || cam.Y < 0 {
		t.Errorf("地図の外を映している: %+v", cam)
	}

	w.Player = Vec{X: 9.5, Y: 9.5}
	cam = w.Camera(4, 4)
	if cam.X > 6 || cam.Y > 6 {
		t.Errorf("南東で地図の外を映している: %+v", cam)
	}

	// 視界が地図より広いときは中央寄せになる。
	cam = w.Camera(20, 20)
	if cam.X != -5 || cam.Y != -5 {
		t.Errorf("中央寄せになっていない: %+v", cam)
	}
}

func TestLatLonRoundsToGrid(t *testing.T) {
	w := testWorld(t)
	w.Player = Vec{X: 0, Y: float64(w.Grid.Rows)}
	got := w.LatLon()
	if math.Abs(got.Lat-w.Grid.OriginLat) > 1e-9 || math.Abs(got.Lon-w.Grid.OriginLon) > 1e-9 {
		t.Errorf("南西端が原点にならない: %+v", got)
	}
}

func TestNearestLandmark(t *testing.T) {
	w := testWorld(t)
	w.Player = Vec{X: 5.5, Y: 7.5}
	if got := w.NearestLandmark(3); got == nil || got.ID != "home" {
		t.Errorf("近傍のランドマークが見つからない: %+v", got)
	}
	if got := w.NearestLandmark(1); got != nil {
		t.Errorf("範囲外のランドマークが返った: %+v", got)
	}
}

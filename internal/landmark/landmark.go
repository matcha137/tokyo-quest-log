// Package landmark はワールドマップ上の目印（町・山・港など）を扱う。
//
// 地形が標高から機械的に決まるのに対し、ランドマークは手で選んで配置する。
// どの場所を「町」として置くかがゲームデザインそのものなので、自動生成しない。
package landmark

import (
	"encoding/json"
	"fmt"
	"io"
	"sort"

	"tokyo-quest-log/internal/geomesh"
	"tokyo-quest-log/internal/worldgrid"
)

// Kind はランドマークの種別。マップ上の記号と、入ったときの挙動を決める。
type Kind string

const (
	KindCity     Kind = "city"     // 大きな町。拠点になる
	KindTown     Kind = "town"     // 町
	KindPeak     Kind = "peak"     // 山頂
	KindDungeon  Kind = "dungeon"  // 洞窟・迷宮
	KindPort     Kind = "port"     // 港。海路の起点
	KindShrine   Kind = "shrine"   // 社寺
	KindFacility Kind = "facility" // その他の施設
)

var validKinds = map[Kind]bool{
	KindCity: true, KindTown: true, KindPeak: true,
	KindDungeon: true, KindPort: true, KindShrine: true, KindFacility: true,
}

// Landmark は1つの目印。座標は実在地点のおおよその緯度経度。
type Landmark struct {
	ID   string  `json:"id"`
	Name string  `json:"name"`
	Kind Kind    `json:"kind"`
	Lat  float64 `json:"lat"`
	Lon  float64 `json:"lon"`
	Note string  `json:"note,omitempty"`
}

type file struct {
	Landmarks []Landmark `json:"landmarks"`
}

// Load はJSONからランドマーク一覧を読む。IDの重複と不正な種別を弾く。
func Load(r io.Reader) ([]Landmark, error) {
	var doc file
	if err := json.NewDecoder(r).Decode(&doc); err != nil {
		return nil, fmt.Errorf("ランドマークの解析: %w", err)
	}
	if len(doc.Landmarks) == 0 {
		return nil, fmt.Errorf("ランドマークが1件もありません")
	}
	seen := make(map[string]bool, len(doc.Landmarks))
	for i, l := range doc.Landmarks {
		switch {
		case l.ID == "":
			return nil, fmt.Errorf("%d番目: id が空です", i+1)
		case seen[l.ID]:
			return nil, fmt.Errorf("id が重複しています: %q", l.ID)
		case l.Name == "":
			return nil, fmt.Errorf("%s: name が空です", l.ID)
		case !validKinds[l.Kind]:
			return nil, fmt.Errorf("%s: 未知の kind: %q", l.ID, l.Kind)
		}
		seen[l.ID] = true
	}
	return doc.Landmarks, nil
}

// Placed はグリッド上に配置済みのランドマーク。
type Placed struct {
	Landmark
	Row int
	Col int
}

// Place はランドマークをグリッドのマスへ対応づける。
// グリッドの範囲外にあるものは skipped に名前を集めて返す。
func Place(g *worldgrid.Grid, landmarks []Landmark) (placed []Placed, skipped []string) {
	for _, l := range landmarks {
		row, col, ok := cellOf(g, l)
		if !ok {
			skipped = append(skipped, l.Name)
			continue
		}
		placed = append(placed, Placed{Landmark: l, Row: row, Col: col})
	}
	sort.Slice(placed, func(i, j int) bool {
		if placed[i].Row != placed[j].Row {
			return placed[i].Row < placed[j].Row
		}
		return placed[i].Col < placed[j].Col
	})
	return placed, skipped
}

func cellOf(g *worldgrid.Grid, l Landmark) (row, col int, ok bool) {
	col = int((l.Lon - g.OriginLon) / g.LonSpan)
	row = g.Rows - 1 - int((l.Lat-g.OriginLat)/g.LatSpan)
	if row < 0 || row >= g.Rows || col < 0 || col >= g.Cols {
		return 0, 0, false
	}
	return row, col, true
}

// MeshCode はランドマークが載る5次メッシュのコードを返す。
// 実データと突き合わせるときの手掛かりになる。
func (l Landmark) MeshCode() (string, error) {
	return geomesh.Encode(geomesh.LatLon{Lat: l.Lat, Lon: l.Lon}, 5)
}

// Index は配置済みランドマークをマス単位で引ける表にする。
type Index map[[2]int]*Placed

// NewIndex は配置済みランドマークから索引を作る。
func NewIndex(placed []Placed) Index {
	idx := make(Index, len(placed))
	for i := range placed {
		idx[[2]int{placed[i].Row, placed[i].Col}] = &placed[i]
	}
	return idx
}

// At は指定マスのランドマークを返す。無ければ nil。
func (i Index) At(row, col int) *Placed {
	return i[[2]int{row, col}]
}

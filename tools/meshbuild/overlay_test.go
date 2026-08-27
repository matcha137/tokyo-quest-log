package main

import (
	"os"
	"path/filepath"
	"testing"

	"tokyo-quest-log/internal/worldgrid"
)

func TestOverlayListSet(t *testing.T) {
	var list overlayList
	if err := list.Set("water=rivers.geojson"); err != nil {
		t.Fatal(err)
	}
	if err := list.Set("内水面=lakes.geojson"); err != nil {
		t.Fatal(err)
	}
	if len(list) != 2 {
		t.Fatalf("要素数 = %d, want 2", len(list))
	}
	if list[0].terrain != worldgrid.Water || list[0].path != "rivers.geojson" {
		t.Errorf("1件目が不正: %+v", list[0])
	}
	// 指定順が保たれること。重なりの優先順位がこれで決まる。
	if list[1].path != "lakes.geojson" {
		t.Errorf("順序が保たれていない: %+v", list)
	}

	for _, bad := range []string{"rivers.geojson", "unknown=x.geojson", "water=", ""} {
		var l overlayList
		if err := l.Set(bad); err == nil {
			t.Errorf("Set(%q) がエラーにならなかった", bad)
		}
	}
}

func writeTemp(t *testing.T, name, content string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), name)
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	return path
}

// 全面を陸地で埋めたグリッドを作る。重ね合わせの効果を数えやすくする。
func filledGrid(t *testing.T) *worldgrid.Grid {
	t.Helper()
	g, err := worldgrid.New(worldgrid.Bounds{MinLat: 35.60, MinLon: 139.60, MaxLat: 35.66, MaxLon: 139.70}, 5)
	if err != nil {
		t.Fatal(err)
	}
	for i := range g.Tiles {
		g.Tiles[i] = worldgrid.Plain
	}
	return g
}

const boundaryDoc = `{"type":"Feature","properties":{},"geometry":{"type":"Polygon","coordinates":[[
 [139.62,35.62],[139.68,35.62],[139.68,35.64],[139.62,35.64],[139.62,35.62]]]}}`

func TestApplyBoundaryMarksOutside(t *testing.T) {
	g := filledGrid(t)
	note, err := applyBoundary(g, writeTemp(t, "boundary.geojson", boundaryDoc))
	if err != nil {
		t.Fatal(err)
	}
	counts := g.Histogram()
	if counts[worldgrid.OutOfArea] == 0 {
		t.Error("都域外が1マスも塗られていない")
	}
	if counts[worldgrid.Plain] == 0 {
		t.Error("境界の内側まで塗り潰されている")
	}
	if note == "" {
		t.Error("結果の説明が空")
	}
	t.Log(note)
}

func TestApplyOverlayDrawsWater(t *testing.T) {
	g := filledGrid(t)
	doc := `{"type":"FeatureCollection","features":[
	 {"type":"Feature","properties":{},"geometry":{"type":"LineString","coordinates":[
	   [139.61,35.63],[139.69,35.63]]}},
	 {"type":"Feature","properties":{},"geometry":{"type":"Polygon","coordinates":[[
	   [139.63,35.605],[139.65,35.605],[139.65,35.615],[139.63,35.615],[139.63,35.605]]]}}]}`
	req := overlayRequest{terrain: worldgrid.Water, path: writeTemp(t, "water.geojson", doc)}
	note, err := applyOverlay(g, req, 2)
	if err != nil {
		t.Fatal(err)
	}
	if g.Histogram()[worldgrid.Water] == 0 {
		t.Fatal("内水面が1マスも塗られていない")
	}
	t.Log(note)
}

// 境界を先に適用した場合、内水面は都域外へはみ出さないこと。
func TestWaterDoesNotLeaveBoundary(t *testing.T) {
	g := filledGrid(t)
	if _, err := applyBoundary(g, writeTemp(t, "boundary.geojson", boundaryDoc)); err != nil {
		t.Fatal(err)
	}
	outsideBefore := g.Histogram()[worldgrid.OutOfArea]

	// 境界をまたいで西から東へ伸びる線。
	doc := `{"type":"LineString","coordinates":[[139.60,35.63],[139.70,35.63]]}`
	req := overlayRequest{terrain: worldgrid.Water, path: writeTemp(t, "river.geojson", doc)}
	if _, err := applyOverlay(g, req, 1); err != nil {
		t.Fatal(err)
	}

	if got := g.Histogram()[worldgrid.OutOfArea]; got != outsideBefore {
		t.Errorf("都域外が %d から %d に変化した。水域がはみ出している", outsideBefore, got)
	}
	if g.Histogram()[worldgrid.Water] == 0 {
		t.Error("境界内にも水域が描かれていない")
	}
}

func TestOverlayErrors(t *testing.T) {
	g := filledGrid(t)
	if _, err := applyOverlay(g, overlayRequest{terrain: worldgrid.Water, path: "存在しない.geojson"}, 1); err == nil {
		t.Error("存在しないファイルがエラーにならなかった")
	}
	empty := writeTemp(t, "empty.geojson", `{"type":"Point","coordinates":[139.7,35.6]}`)
	if _, err := applyOverlay(g, overlayRequest{terrain: worldgrid.Water, path: empty}, 1); err == nil {
		t.Error("面も線も無いGeoJSONがエラーにならなかった")
	}
	lineOnly := writeTemp(t, "line.geojson", `{"type":"LineString","coordinates":[[139.6,35.6],[139.7,35.65]]}`)
	if _, err := applyBoundary(g, lineOnly); err == nil {
		t.Error("面を含まない境界がエラーにならなかった")
	}
}

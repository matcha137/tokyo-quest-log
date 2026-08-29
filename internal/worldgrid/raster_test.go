package worldgrid

import (
	"testing"

	"tokyo-quest-log/internal/geojson"
)

// unitGrid は1マス1度・10×10のグリッド。gridPos が x=lon, y=10-lat に
// なるため、期待値を手で確かめられる。
func unitGrid() *Grid {
	return &Grid{
		Level: 5, OriginLat: 0, OriginLon: 0,
		LatSpan: 1, LonSpan: 1, Cols: 10, Rows: 10,
		Tiles: make([]Terrain, 100),
	}
}

func rect(minLon, minLat, maxLon, maxLat float64) geojson.Ring {
	return geojson.Ring{
		{Lon: minLon, Lat: minLat}, {Lon: maxLon, Lat: minLat},
		{Lon: maxLon, Lat: maxLat}, {Lon: minLon, Lat: maxLat},
		{Lon: minLon, Lat: minLat},
	}
}

func countOf(g *Grid, t Terrain) int { return g.Histogram()[t] }

func TestFillPolygonsCoversExpectedCells(t *testing.T) {
	g := unitGrid()
	// 経度2〜6・緯度2〜6の正方形。中心が区間に入るのは行4〜7・列2〜5。
	painted := g.FillPolygons([]geojson.Polygon{{rect(2, 2, 6, 6)}}, Water)
	if painted != 16 {
		t.Errorf("塗ったマス数 = %d, want 16", painted)
	}
	for row := 0; row < g.Rows; row++ {
		for col := 0; col < g.Cols; col++ {
			want := row >= 4 && row <= 7 && col >= 2 && col <= 5
			if got := g.At(row, col) == Water; got != want {
				t.Errorf("(%d,%d) = %s, 内側であるべき=%v", row, col, g.At(row, col), want)
			}
		}
	}
}

// Polygon の2番目以降の輪は穴として抜けること。
func TestFillPolygonsLeavesHole(t *testing.T) {
	g := unitGrid()
	poly := geojson.Polygon{rect(1, 1, 9, 9), rect(3, 3, 7, 7)}
	painted := g.FillPolygons([]geojson.Polygon{poly}, Water)
	if painted != 48 {
		t.Errorf("塗ったマス数 = %d, want 48（8×8から穴4×4を除く）", painted)
	}
	for row := 3; row <= 6; row++ {
		for col := 3; col <= 6; col++ {
			if g.At(row, col) == Water {
				t.Errorf("(%d,%d) は穴のはずが塗られている", row, col)
			}
		}
	}
}

func TestMaskOutside(t *testing.T) {
	g := unitGrid()
	painted := g.MaskOutside([]geojson.Polygon{{rect(2, 2, 6, 6)}}, OutOfArea)
	if painted != 84 {
		t.Errorf("塗ったマス数 = %d, want 84（100 - 内側16）", painted)
	}
	if g.At(5, 3) != Sea {
		t.Errorf("内側 (5,3) が書き換えられている: %s", g.At(5, 3))
	}
	if g.At(0, 0) != OutOfArea {
		t.Errorf("外側 (0,0) = %s, want 都域外", g.At(0, 0))
	}
}

func TestStrokeLinesFollowsPath(t *testing.T) {
	g := unitGrid()
	line := geojson.Line{{Lon: 1, Lat: 5.5}, {Lon: 8, Lat: 5.5}}
	painted := g.StrokeLines([]geojson.Line{line}, 1, Water, false)
	if painted < 7 {
		t.Errorf("塗ったマス数 = %d, 7以上を期待", painted)
	}
	for row := 0; row < g.Rows; row++ {
		for col := 0; col < g.Cols; col++ {
			if g.At(row, col) == Water && row != 4 {
				t.Errorf("(%d,%d) が行4以外に塗られている", row, col)
			}
		}
	}
	for col := 1; col <= 7; col++ {
		if g.At(4, col) != Water {
			t.Errorf("(4,%d) が塗られていない", col)
		}
	}
}

// 線に幅を持たせると隣接する行にも広がること。
func TestStrokeLinesWidth(t *testing.T) {
	g := unitGrid()
	line := geojson.Line{{Lon: 1, Lat: 5.5}, {Lon: 8, Lat: 5.5}}
	g.StrokeLines([]geojson.Line{line}, 3, Water, false)
	for _, row := range []int{3, 4, 5} {
		if g.At(row, 4) != Water {
			t.Errorf("幅3の線が行%dに届いていない", row)
		}
	}
	if g.At(1, 4) == Water {
		t.Error("幅3の線が行1まで広がっている")
	}
}

// onlyLand を指定すると海と都域外は塗らないこと。河川が海上へ伸びるのを防ぐ。
func TestStrokeLinesOnlyLand(t *testing.T) {
	g := unitGrid()
	for col := 0; col < g.Cols; col++ {
		g.Set(4, col, Plain)
	}
	g.Set(4, 5, Sea)
	g.Set(4, 6, OutOfArea)

	g.StrokeLines([]geojson.Line{{{Lon: 1, Lat: 5.5}, {Lon: 8, Lat: 5.5}}}, 1, Water, true)

	if g.At(4, 5) != Sea {
		t.Errorf("(4,5) の海が塗り潰された: %s", g.At(4, 5))
	}
	if g.At(4, 6) != OutOfArea {
		t.Errorf("(4,6) の都域外が塗り潰された: %s", g.At(4, 6))
	}
	if g.At(4, 4) != Water {
		t.Errorf("(4,4) の陸地が塗られていない: %s", g.At(4, 4))
	}
}

// 範囲外の形状を渡しても落ちず、何も塗らないこと。
func TestRasterIgnoresOutOfRange(t *testing.T) {
	g := unitGrid()
	g.FillPolygons([]geojson.Polygon{{rect(100, 100, 110, 110)}}, Water)
	g.StrokeLines([]geojson.Line{{{Lon: -50, Lat: -50}, {Lon: -40, Lat: -40}}}, 2, Water, false)
	if got := countOf(g, Water); got != 0 {
		t.Errorf("範囲外の形状で %d マス塗られた", got)
	}
}

func TestTerrainFromName(t *testing.T) {
	for name, want := range map[string]Terrain{
		"sea": Sea, "Water": Water, "水": 0, "内水面": Water, "OUTOFAREA": OutOfArea, "高山": HighMountain,
	} {
		got, err := TerrainFromName(name)
		if name == "水" {
			if err == nil {
				t.Errorf("TerrainFromName(%q) がエラーにならなかった", name)
			}
			continue
		}
		if err != nil {
			t.Errorf("TerrainFromName(%q): %v", name, err)
			continue
		}
		if got != want {
			t.Errorf("TerrainFromName(%q) = %s, want %s", name, got, want)
		}
	}
}

// preserve に挙げた地形は境界の外でも塗り替えられないこと。
// 行政区域の面は陸地しか覆わないため、海を残す用途で使う。
func TestMaskOutsidePreservesTerrain(t *testing.T) {
	g := unitGrid()
	for i := range g.Tiles {
		g.Tiles[i] = Plain
	}
	for col := 0; col < g.Cols; col++ {
		g.Set(0, col, Sea) // 北端をすべて海にする
	}
	painted := g.MaskOutside([]geojson.Polygon{{rect(2, 2, 6, 6)}}, OutOfArea, Sea)

	for col := 0; col < g.Cols; col++ {
		if g.At(0, col) != Sea {
			t.Fatalf("(0,%d) の海が塗り替えられた: %s", col, g.At(0, col))
		}
	}
	if want := 84 - g.Cols; painted != want {
		t.Errorf("塗ったマス数 = %d, want %d（外側84から北端の海10を除く）", painted, want)
	}
	if g.At(1, 0) != OutOfArea {
		t.Errorf("海でない外側が塗られていない: %s", g.At(1, 0))
	}
}

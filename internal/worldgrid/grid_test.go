package worldgrid

import (
	"bytes"
	"testing"

	"tokyo-quest-log/internal/geomesh"
)

func TestNewCoversRequestedBounds(t *testing.T) {
	g, err := New(TokyoMainland, 5)
	if err != nil {
		t.Fatal(err)
	}
	if g.OriginLat > TokyoMainland.MinLat || g.OriginLon > TokyoMainland.MinLon {
		t.Errorf("原点が要求範囲の内側にある: (%v, %v)", g.OriginLat, g.OriginLon)
	}
	north := g.OriginLat + float64(g.Rows)*g.LatSpan
	east := g.OriginLon + float64(g.Cols)*g.LonSpan
	if north < TokyoMainland.MaxLat || east < TokyoMainland.MaxLon {
		t.Errorf("北東端 (%v, %v) が要求範囲を覆っていない", north, east)
	}
	t.Logf("グリッド %d × %d マス", g.Cols, g.Rows)
}

// グリッドがメッシュ境界に揃っていれば、全マスの中心は互いに異なる
// メッシュコードへ対応し、かつそのコードの区画とマスが一致する。
func TestCellCenterMapsToUniqueMesh(t *testing.T) {
	g, err := New(Bounds{MinLat: 35.60, MinLon: 139.70, MaxLat: 35.70, MaxLon: 139.80}, 5)
	if err != nil {
		t.Fatal(err)
	}
	seen := make(map[string]struct{}, g.Cols*g.Rows)
	for row := 0; row < g.Rows; row++ {
		for col := 0; col < g.Cols; col++ {
			center := g.CellCenter(row, col)
			code, err := geomesh.Encode(center, g.Level)
			if err != nil {
				t.Fatalf("(%d,%d): %v", row, col, err)
			}
			if _, dup := seen[code]; dup {
				t.Fatalf("(%d,%d): メッシュコード %s が重複", row, col, code)
			}
			seen[code] = struct{}{}

			cell, err := geomesh.Decode(code)
			if err != nil {
				t.Fatal(err)
			}
			if diff := cell.LatSpan - g.LatSpan; diff > 1e-12 || diff < -1e-12 {
				t.Fatalf("マスとメッシュの緯度幅が不一致: %v vs %v", g.LatSpan, cell.LatSpan)
			}
		}
	}
	if len(seen) != g.Cols*g.Rows {
		t.Errorf("一意なメッシュ数 %d, マス数 %d", len(seen), g.Cols*g.Rows)
	}
}

// 行0が北端、列0が西端であること。
func TestCellCenterOrientation(t *testing.T) {
	g, err := New(TokyoMainland, 5)
	if err != nil {
		t.Fatal(err)
	}
	northWest := g.CellCenter(0, 0)
	southEast := g.CellCenter(g.Rows-1, g.Cols-1)
	if northWest.Lat <= southEast.Lat {
		t.Errorf("行0が北端になっていない: %v vs %v", northWest.Lat, southEast.Lat)
	}
	if northWest.Lon >= southEast.Lon {
		t.Errorf("列0が西端になっていない: %v vs %v", northWest.Lon, southEast.Lon)
	}
}

func TestGridBinaryRoundTrip(t *testing.T) {
	g, err := New(Bounds{MinLat: 35.60, MinLon: 139.70, MaxLat: 35.65, MaxLon: 139.75}, 5)
	if err != nil {
		t.Fatal(err)
	}
	for i := range g.Tiles {
		g.Tiles[i] = Terrain(i % int(TerrainCount))
	}

	var buf bytes.Buffer
	n, err := g.WriteTo(&buf)
	if err != nil {
		t.Fatal(err)
	}
	if n != int64(buf.Len()) {
		t.Errorf("WriteTo が返した長さ %d, 実際 %d", n, buf.Len())
	}

	got, err := ReadFrom(&buf)
	if err != nil {
		t.Fatal(err)
	}
	if got.Cols != g.Cols || got.Rows != g.Rows || got.Level != g.Level {
		t.Fatalf("形状が不一致: %+v", got)
	}
	if got.OriginLat != g.OriginLat || got.OriginLon != g.OriginLon {
		t.Errorf("原点が不一致: (%v,%v)", got.OriginLat, got.OriginLon)
	}
	for i := range g.Tiles {
		if got.Tiles[i] != g.Tiles[i] {
			t.Fatalf("タイル %d が不一致: %v want %v", i, got.Tiles[i], g.Tiles[i])
		}
	}
}

func TestReadFromRejectsGarbage(t *testing.T) {
	if _, err := ReadFrom(bytes.NewReader([]byte("NOPE0000"))); err == nil {
		t.Error("不正なマジックがエラーにならなかった")
	}
}

func TestWalkable(t *testing.T) {
	for _, tc := range []struct {
		terrain Terrain
		want    bool
	}{
		{Sea, false}, {HighMountain, false},
		{Lowland, true}, {Plain, true}, {Plateau, true}, {Hill, true}, {Mountain, true},
	} {
		if got := tc.terrain.Walkable(); got != tc.want {
			t.Errorf("%s.Walkable() = %v, want %v", tc.terrain, got, tc.want)
		}
	}
}

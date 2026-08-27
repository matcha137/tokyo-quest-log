package main

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"tokyo-quest-log/internal/geomesh"
	"tokyo-quest-log/internal/worldgrid"
)

// testBounds は検証用の小さな範囲。実行時間を抑えるため都域全体は使わない。
var testBounds = worldgrid.Bounds{MinLat: 35.60, MinLon: 139.60, MaxLat: 35.66, MaxLon: 139.70}

// writeFixtureCSV は範囲の西半分だけに標高を持つ合成CSVを書き出す。
// 東半分はデータ欠損＝海になるはずで、海岸線の再現を検証できる。
func writeFixtureCSV(t *testing.T, g *worldgrid.Grid) string {
	t.Helper()
	var buf bytes.Buffer
	buf.WriteString("meshcode,elevation\n")
	midLon := (testBounds.MinLon + testBounds.MaxLon) / 2
	for row := 0; row < g.Rows; row++ {
		for col := 0; col < g.Cols; col++ {
			center := g.CellCenter(row, col)
			if center.Lon >= midLon {
				continue
			}
			code, err := geomesh.Encode(center, 5)
			if err != nil {
				t.Fatal(err)
			}
			// 北ほど高い勾配にして、全地形帯が出現するようにする。
			// 行インデックス基準にすることで、最南端の行を確実に標高0にする。
			elevation := float64(g.Rows-1-row) / float64(g.Rows-1) * 1200
			fmt.Fprintf(&buf, "%s,%.1f\n", code, elevation)
		}
	}
	path := filepath.Join(t.TempDir(), "elevation.csv")
	if err := os.WriteFile(path, buf.Bytes(), 0o644); err != nil {
		t.Fatal(err)
	}
	return path
}

func buildTestGrid(t *testing.T) *worldgrid.Grid {
	t.Helper()
	g, err := worldgrid.New(testBounds, 5)
	if err != nil {
		t.Fatal(err)
	}
	table, err := loadElevationCSV(writeFixtureCSV(t, g))
	if err != nil {
		t.Fatal(err)
	}
	for row := 0; row < g.Rows; row++ {
		for col := 0; col < g.Cols; col++ {
			if elevation, ok := table.lookup(g.CellCenter(row, col)); ok {
				g.Set(row, col, classify(elevation))
			}
		}
	}
	return g
}

// 標高データが無いマスが海になること。国土数値情報の標高メッシュは
// 陸域のみを収録するため、この規則で海岸線が決まる。
func TestMissingElevationBecomesSea(t *testing.T) {
	g := buildTestGrid(t)
	midLon := (testBounds.MinLon + testBounds.MaxLon) / 2

	var landInEast, seaInWest int
	for row := 0; row < g.Rows; row++ {
		for col := 0; col < g.Cols; col++ {
			east := g.CellCenter(row, col).Lon >= midLon
			sea := g.At(row, col) == worldgrid.Sea
			if east && !sea {
				landInEast++
			}
			if !east && sea {
				seaInWest++
			}
		}
	}
	if landInEast != 0 {
		t.Errorf("データが無い東側に陸が %d マスある", landInEast)
	}
	if seaInWest != 0 {
		t.Errorf("データがある西側に海が %d マスある", seaInWest)
	}
}

// 標高の勾配が地形帯として並ぶこと。
func TestElevationBandsAppear(t *testing.T) {
	g := buildTestGrid(t)
	counts := g.Histogram()
	for _, terrain := range []worldgrid.Terrain{
		worldgrid.Lowland, worldgrid.Plain, worldgrid.Plateau,
		worldgrid.Hill, worldgrid.Mountain, worldgrid.HighMountain,
	} {
		if counts[terrain] == 0 {
			t.Errorf("%s が1マスも生成されなかった", terrain)
		}
	}
	// 南が低く北が高い勾配なので、最南端の行は最北端の行より低い地形になる。
	if g.At(g.Rows-1, 0) >= g.At(0, 0) {
		t.Errorf("南端 %s が北端 %s より高い地形になっている", g.At(g.Rows-1, 0), g.At(0, 0))
	}
}

func TestClassifyBoundaries(t *testing.T) {
	cases := []struct {
		elevation float64
		want      worldgrid.Terrain
	}{
		{-2, worldgrid.Lowland}, // 海抜0m地帯は陸
		{4.9, worldgrid.Lowland},
		{5, worldgrid.Plain},
		{49.9, worldgrid.Plain},
		{50, worldgrid.Plateau},
		{150, worldgrid.Hill},
		{599, worldgrid.Mountain}, // 高尾山
		{900, worldgrid.HighMountain},
		{2017, worldgrid.HighMountain}, // 雲取山
	}
	for _, tc := range cases {
		if got := classify(tc.elevation); got != tc.want {
			t.Errorf("classify(%.1f) = %s, want %s", tc.elevation, got, tc.want)
		}
	}
}

// 生成したグリッドを保存して読み戻せること、プレビューが出力できること。
func TestWriteGridAndPreview(t *testing.T) {
	g := buildTestGrid(t)
	dir := t.TempDir()
	binPath := filepath.Join(dir, "world.bin")
	pngPath := filepath.Join(dir, "preview.png")

	if err := writeGrid(g, binPath); err != nil {
		t.Fatal(err)
	}
	if err := writePreviewPNG(g, nil, pngPath, 2); err != nil {
		t.Fatal(err)
	}

	f, err := os.Open(binPath)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	got, err := worldgrid.ReadFrom(f)
	if err != nil {
		t.Fatal(err)
	}
	if got.Cols != g.Cols || got.Rows != g.Rows {
		t.Fatalf("読み戻した形状が不一致: %d×%d", got.Cols, got.Rows)
	}
	for i := range g.Tiles {
		if got.Tiles[i] != g.Tiles[i] {
			t.Fatalf("タイル %d が不一致", i)
		}
	}
	if info, err := os.Stat(pngPath); err != nil || info.Size() == 0 {
		t.Errorf("プレビューPNGが生成されていない: %v", err)
	}
}

func TestParseBounds(t *testing.T) {
	b, err := parseBounds("35.5, 138.9, 35.9, 139.9")
	if err != nil {
		t.Fatal(err)
	}
	if b.MinLat != 35.5 || b.MaxLon != 139.9 {
		t.Errorf("parseBounds = %+v", b)
	}
	for _, bad := range []string{"", "1,2,3", "a,2,3,4", "35.9,138.9,35.5,139.9"} {
		if _, err := parseBounds(bad); err == nil {
			t.Errorf("parseBounds(%q) がエラーにならなかった", bad)
		}
	}
}

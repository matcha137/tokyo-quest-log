// meshbuild は標高メッシュのCSVから、東京都のワールドマップ用タイルグリッドを生成する。
//
//	go run ./tools/meshbuild -in data/elevation.csv -out assets/tokyo_mainland.bin -preview preview.png
//
// 入力CSVは「メッシュコード,標高(m)」の2列。3次〜5次メッシュのコードを受け付け、
// 次数が混在していても細かいものを優先して使う。
package main

import (
	"bufio"
	"flag"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"tokyo-quest-log/internal/worldgrid"
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, "meshbuild:", err)
		os.Exit(1)
	}
}

func run() error {
	var (
		in         = flag.String("in", "", "標高メッシュCSVのパス（必須）")
		out        = flag.String("out", "assets/tokyo_mainland.bin", "生成するグリッドの出力先")
		preview    = flag.String("preview", "", "確認用PNGの出力先（空なら生成しない）")
		level      = flag.Int("level", 5, "出力するメッシュ次数（5=約250m, 4=約500m, 3=約1km）")
		scale      = flag.Int("scale", 3, "プレビュー1タイルあたりのピクセル数")
		boundsFlag = flag.String("bounds", "", "対象範囲 minLat,minLon,maxLat,maxLon（既定は東京都本土）")
	)
	flag.Parse()

	if *in == "" {
		flag.Usage()
		return fmt.Errorf("-in は必須です")
	}
	bounds := worldgrid.TokyoMainland
	if *boundsFlag != "" {
		parsed, err := parseBounds(*boundsFlag)
		if err != nil {
			return err
		}
		bounds = parsed
	}

	table, err := loadElevationCSV(*in)
	if err != nil {
		return err
	}
	fmt.Printf("標高メッシュを読み込みました:\n%s", table.summary())

	grid, err := worldgrid.New(bounds, *level)
	if err != nil {
		return err
	}

	var minElev, maxElev = math.Inf(1), math.Inf(-1)
	for row := 0; row < grid.Rows; row++ {
		for col := 0; col < grid.Cols; col++ {
			elevation, ok := table.lookup(grid.CellCenter(row, col))
			if !ok {
				continue // 陸域データなし = 海のまま
			}
			minElev = math.Min(minElev, elevation)
			maxElev = math.Max(maxElev, elevation)
			grid.Set(row, col, classify(elevation))
		}
	}

	if err := writeGrid(grid, *out); err != nil {
		return err
	}
	if *preview != "" {
		if err := os.MkdirAll(filepath.Dir(*preview), 0o755); err != nil {
			return err
		}
		if err := writePreviewPNG(grid, *preview, *scale); err != nil {
			return err
		}
	}

	report(grid, *out, *preview, minElev, maxElev)
	return nil
}

func writeGrid(g *worldgrid.Grid, path string) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	defer f.Close()
	w := bufio.NewWriter(f)
	if _, err := g.WriteTo(w); err != nil {
		return err
	}
	return w.Flush()
}

func report(g *worldgrid.Grid, out, preview string, minElev, maxElev float64) {
	latMeters := g.LatSpan * 111_320
	lonMeters := g.LonSpan * 111_320 * math.Cos((g.OriginLat+float64(g.Rows)*g.LatSpan/2)*math.Pi/180)

	fmt.Printf("\nワールドマップを生成しました\n")
	fmt.Printf("  グリッド : %d × %d = %d マス（%d次メッシュ）\n", g.Cols, g.Rows, g.Cols*g.Rows, g.Level)
	fmt.Printf("  1マス    : 南北 %.0fm × 東西 %.0fm\n", latMeters, lonMeters)
	fmt.Printf("  実寸     : 東西 %.1fkm × 南北 %.1fkm\n",
		float64(g.Cols)*lonMeters/1000, float64(g.Rows)*latMeters/1000)
	if !math.IsInf(minElev, 1) {
		fmt.Printf("  標高範囲 : %.0fm 〜 %.0fm\n", minElev, maxElev)
	}

	fmt.Printf("\n地形の区分:\n%s", describeBands())

	counts := g.Histogram()
	total := g.Cols * g.Rows
	fmt.Printf("\n地形の内訳:\n")
	for t := worldgrid.Terrain(0); t < worldgrid.TerrainCount; t++ {
		share := float64(counts[t]) / float64(total) * 100
		fmt.Printf("  %-4s %7d マス (%5.1f%%) %s\n", t, counts[t], share, bar(share))
	}
	land := total - counts[worldgrid.Sea]
	fmt.Printf("\n  陸地 %d マス = 約 %.0f km²\n", land, float64(land)*latMeters*lonMeters/1e6)

	if info, err := os.Stat(out); err == nil {
		fmt.Printf("\n  %s (%.1f KB)\n", out, float64(info.Size())/1024)
	}
	if preview != "" {
		fmt.Printf("  %s\n", preview)
	}
}

func bar(share float64) string {
	return strings.Repeat("█", int(share/2))
}

func parseBounds(s string) (worldgrid.Bounds, error) {
	parts := strings.Split(s, ",")
	if len(parts) != 4 {
		return worldgrid.Bounds{}, fmt.Errorf("-bounds は minLat,minLon,maxLat,maxLon 形式: %q", s)
	}
	values := make([]float64, 4)
	for i, part := range parts {
		v, err := strconv.ParseFloat(strings.TrimSpace(part), 64)
		if err != nil {
			return worldgrid.Bounds{}, fmt.Errorf("-bounds の %d 番目が数値でない: %q", i+1, part)
		}
		values[i] = v
	}
	b := worldgrid.Bounds{MinLat: values[0], MinLon: values[1], MaxLat: values[2], MaxLon: values[3]}
	if b.MinLat >= b.MaxLat || b.MinLon >= b.MaxLon {
		return worldgrid.Bounds{}, fmt.Errorf("-bounds の最小値が最大値以上: %+v", b)
	}
	return b, nil
}

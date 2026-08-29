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

	"tokyo-quest-log/internal/landmark"
	"tokyo-quest-log/internal/tmx"
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
		boundary   = flag.String("boundary", "", "都域境界のGeoJSON。外側を都域外として塗る")
		lineWidth  = flag.Int("line-width", 1, "線状の水域をなぞる幅（マス）")
		landmarks  = flag.String("landmarks", "", "ランドマークJSONのパス")
		tmxOut     = flag.String("tmx", "", "Tiled形式(.tmx)の出力先。タイルセット画像も隣に書き出す")
		tileSize   = flag.Int("tile-size", 32, "TMXの1マスのピクセル数")
		overlays   overlayList
	)
	flag.Var(&overlays, "overlay", "重ね合わせる形状 地形名=GeoJSONのパス（繰り返し指定可、指定順に適用）")
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

	// 境界を先に適用する。都域外を確定させてから水域を描くことで、
	// 河川が都外へはみ出して描かれるのを防ぐ。
	var notes []string
	if *boundary != "" {
		note, err := applyBoundary(grid, *boundary)
		if err != nil {
			return err
		}
		notes = append(notes, note)
	}
	for _, req := range overlays {
		note, err := applyOverlay(grid, req, *lineWidth)
		if err != nil {
			return err
		}
		notes = append(notes, note)
	}

	// ランドマークは地形の上に載る情報なので、重ね合わせの後に配置する。
	var placed []landmark.Placed
	if *landmarks != "" {
		loaded, err := loadLandmarks(*landmarks)
		if err != nil {
			return err
		}
		var skipped []string
		placed, skipped = landmark.Place(grid, loaded)
		note := fmt.Sprintf("  ランドマーク %s: %d 件を配置", *landmarks, len(placed))
		if len(skipped) > 0 {
			note += fmt.Sprintf("（範囲外のため除外: %s）", strings.Join(skipped, ", "))
		}
		notes = append(notes, note)
		if stranded := strandedLandmarks(grid, placed); len(stranded) > 0 {
			notes = append(notes, "  警告: 通行できないマスに載っています: "+strings.Join(stranded, ", "))
		}
	}

	if err := writeGrid(grid, *out); err != nil {
		return err
	}
	if *tmxOut != "" {
		if err := writeTMX(grid, placed, *tmxOut, *tileSize); err != nil {
			return err
		}
	}
	if *preview != "" {
		if err := os.MkdirAll(filepath.Dir(*preview), 0o755); err != nil {
			return err
		}
		if err := writePreviewPNG(grid, placed, *preview, *scale); err != nil {
			return err
		}
	}

	report(grid, *out, *preview, minElev, maxElev, notes)
	if *tmxOut != "" {
		fmt.Println("  " + *tmxOut)
		fmt.Println("  " + tilesetPathFor(*tmxOut))
	}
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

func report(g *worldgrid.Grid, out, preview string, minElev, maxElev float64, notes []string) {
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

	fmt.Printf("\n標高による地形の区分:\n%s", describeBands())

	if len(notes) > 0 {
		fmt.Println()
		fmt.Println("重ね合わせ:")
		for _, note := range notes {
			fmt.Println(note)
		}
	}

	counts := g.Histogram()
	total := g.Cols * g.Rows
	fmt.Printf("\n地形の内訳:\n")
	for t := worldgrid.Terrain(0); t < worldgrid.TerrainCount; t++ {
		share := float64(counts[t]) / float64(total) * 100
		fmt.Printf("  %-4s %7d マス (%5.1f%%) %s\n", t, counts[t], share, bar(share))
	}
	land := total - counts[worldgrid.Sea] - counts[worldgrid.Water] - counts[worldgrid.OutOfArea]
	fmt.Printf("\n  陸地 %d マス = 約 %.0f km²（海・内水面・都域外を除く）\n", land, float64(land)*latMeters*lonMeters/1e6)

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

func loadLandmarks(path string) ([]landmark.Landmark, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	loaded, err := landmark.Load(f)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", path, err)
	}
	return loaded, nil
}

// tilesetPathFor は TMX と同じディレクトリに置くタイルセット画像のパスを返す。
func tilesetPathFor(tmxPath string) string {
	return filepath.Join(filepath.Dir(tmxPath), "terrain.png")
}

func writeTMX(g *worldgrid.Grid, placed []landmark.Placed, path string, tileSize int) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	// TMX はタイルセット画像を相対パスで参照するため、先に画像を書き出す。
	if err := tmx.WriteTilesetPNG(tilesetPathFor(path), tileSize); err != nil {
		return fmt.Errorf("タイルセット画像の書き出し: %w", err)
	}
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	defer f.Close()
	w := bufio.NewWriter(f)
	if err := tmx.Export(w, g, placed, tmx.Options{TileSize: tileSize}); err != nil {
		return err
	}
	return w.Flush()
}

// strandedLandmarks は通行できないマスに載ってしまったランドマークを返す。
// 座標が概算のため、境界や海のマスへずれ込むことがある。到達できない町は
// ゲームとして成立しないので、生成時に気づけるようにする。
func strandedLandmarks(g *worldgrid.Grid, placed []landmark.Placed) []string {
	var stranded []string
	for _, p := range placed {
		if !g.At(p.Row, p.Col).Walkable() {
			stranded = append(stranded, fmt.Sprintf("%s(%s)", p.Name, g.At(p.Row, p.Col)))
		}
	}
	return stranded
}

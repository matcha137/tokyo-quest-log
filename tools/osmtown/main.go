// osmtown は Overpass API で取得した OpenStreetMap のデータを、
// 街を歩く詳細マップのタイルグリッドへ変換する。
//
//	go run ./tools/osmtown -in data/osm/tokyo_station.json -out assets/town_tokyo.bin
//
// ワールドマップと違い、街の骨格は標高では出ない。東京駅周辺の高低差は
// 2km四方で15m程度しかないため、道路・建物・線路・水面・緑地といった
// 人工物を地形として扱う。
//
// 出典: OpenStreetMap contributors（ODbL）
package main

import (
	"bufio"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"tokyo-quest-log/internal/geojson"
	"tokyo-quest-log/internal/worldgrid"
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, "osmtown:", err)
		os.Exit(1)
	}
}

func run() error {
	var (
		in      = flag.String("in", "", "Overpass APIのJSON（必須）")
		out     = flag.String("out", "assets/town_tokyo.bin", "グリッドの出力先")
		preview = flag.String("preview", "", "確認用PNGの出力先")
		level   = flag.Int("level", 10, "メッシュ次数（10で東京付近およそ7m × 9m）")
		scale   = flag.Int("scale", 2, "プレビュー1マスあたりのピクセル数")
		boundsF = flag.String("bounds", "", "対象範囲 minLat,minLon,maxLat,maxLon（既定は取得データの外接矩形）")
	)
	flag.Parse()
	if *in == "" {
		flag.Usage()
		return fmt.Errorf("-in は必須です")
	}

	doc, err := loadOverpass(*in)
	if err != nil {
		return err
	}
	// Overpass は範囲に掛かった way を丸ごと返すため、線路や幹線道路が
	// 範囲外まで伸びる。外接矩形をそのまま使うと意図の数倍の広さになるので、
	// 問い合わせに使った範囲を明示できるようにしている。
	var bounds worldgrid.Bounds
	if *boundsF != "" {
		parsed, err := parseBounds(*boundsF)
		if err != nil {
			return err
		}
		bounds = parsed
	} else {
		var ok bool
		bounds, ok = doc.bounds()
		if !ok {
			return fmt.Errorf("%s: 座標を持つ要素がありません", *in)
		}
	}

	grid, err := worldgrid.New(bounds, *level)
	if err != nil {
		return err
	}
	// 何も無い場所は街区の地面にする。海のままだと街の外側が水没して見える。
	for i := range grid.Tiles {
		grid.Tiles[i] = worldgrid.Ground
	}

	stats := paint(grid, doc)

	if err := writeGrid(grid, *out); err != nil {
		return err
	}
	if *preview != "" {
		if err := writePreviewPNG(grid, *preview, *scale); err != nil {
			return err
		}
	}
	report(grid, stats, bounds, *out, *preview)
	return nil
}

// Overpass API の出力。way のジオメトリが埋め込まれている前提（out geom）。
type overpassDoc struct {
	Elements []element `json:"elements"`
}

type element struct {
	Type     string            `json:"type"`
	ID       int64             `json:"id"`
	Geometry []coord           `json:"geometry"`
	Tags     map[string]string `json:"tags"`
}

type coord struct {
	Lat float64 `json:"lat"`
	Lon float64 `json:"lon"`
}

func loadOverpass(path string) (*overpassDoc, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	var doc overpassDoc
	if err := json.NewDecoder(bufio.NewReaderSize(f, 1<<20)).Decode(&doc); err != nil {
		return nil, fmt.Errorf("%s: %w", path, err)
	}
	if len(doc.Elements) == 0 {
		return nil, fmt.Errorf("%s: 要素がありません", path)
	}
	return &doc, nil
}

func (d *overpassDoc) bounds() (worldgrid.Bounds, bool) {
	b := worldgrid.Bounds{MinLat: 90, MinLon: 180, MaxLat: -90, MaxLon: -180}
	found := false
	for _, e := range d.Elements {
		for _, c := range e.Geometry {
			found = true
			b.MinLat, b.MaxLat = min(b.MinLat, c.Lat), max(b.MaxLat, c.Lat)
			b.MinLon, b.MaxLon = min(b.MinLon, c.Lon), max(b.MaxLon, c.Lon)
		}
	}
	return b, found
}

// layer は塗る順番を決める分類。後の層ほど上に載る。
type layer int

const (
	layerNone layer = iota
	layerPark
	layerWater
	layerRail
	layerRoad
	layerBuilding
)

var layerOrder = []layer{layerPark, layerWater, layerRail, layerRoad, layerBuilding}

var layerTerrain = map[layer]worldgrid.Terrain{
	layerPark:     worldgrid.Park,
	layerWater:    worldgrid.Water,
	layerRail:     worldgrid.Rail,
	layerRoad:     worldgrid.Road,
	layerBuilding: worldgrid.Building,
}

var layerNames = map[layer]string{
	layerPark: "公園", layerWater: "水面", layerRail: "線路",
	layerRoad: "道路", layerBuilding: "建物",
}

// classify は way をどの層に塗るか決める。判定できないものは layerNone。
func classify(tags map[string]string) layer {
	switch {
	case tags["building"] != "" || tags["building:part"] != "":
		return layerBuilding
	case tags["railway"] != "":
		if !surfaceRail(tags) {
			return layerNone
		}
		return layerRail
	case tags["natural"] == "water" || tags["waterway"] != "":
		return layerWater
	case tags["leisure"] == "park" || tags["leisure"] == "garden" || tags["landuse"] == "grass":
		return layerPark
	case tags["highway"] != "":
		return layerRoad
	}
	return layerNone
}

// surfaceRail は地上を走る線路かを返す。
// 都心は地下鉄が縦横に走っており、そのまま壁として塗ると
// 実際には存在しない障害物で街が分断される。
func surfaceRail(tags map[string]string) bool {
	switch tags["railway"] {
	case "subway", "platform", "platform_edge", "abandoned", "razed", "construction", "proposed":
		return false
	}
	if tags["tunnel"] != "" && tags["tunnel"] != "no" {
		return false
	}
	if v, err := strconv.Atoi(tags["layer"]); err == nil && v < 0 {
		return false
	}
	return true
}

// widthOf は線として塗るときの幅（マス）を返す。
func widthOf(g *worldgrid.Grid, tags map[string]string) int {
	meters := 12.0
	switch tags["highway"] {
	case "motorway", "trunk", "primary":
		meters = 30
	case "secondary", "tertiary":
		meters = 18
	case "footway", "path", "steps", "cycleway":
		meters = 4
	}
	if tags["railway"] != "" {
		meters = 12
	}
	if tags["waterway"] != "" {
		meters = 14
	}
	// 経度方向のマス幅を基準に、実寸からマス数へ直す。
	const degreeMeters = 111_320.0
	cellMeters := g.LonSpan * degreeMeters * 0.81 // 東京付近の緯度補正
	return max(1, int(meters/cellMeters+0.5))
}

type paintStats struct {
	ways    map[layer]int
	painted map[layer]int
	skipped int
}

func paint(g *worldgrid.Grid, doc *overpassDoc) paintStats {
	stats := paintStats{ways: map[layer]int{}, painted: map[layer]int{}}

	// 層ごとにまとめてから、決まった順に塗る。
	// 建物を最後にすることで、街区の輪郭がはっきり残る。
	byLayer := map[layer][]element{}
	for _, e := range doc.Elements {
		if len(e.Geometry) < 2 {
			continue
		}
		l := classify(e.Tags)
		if l == layerNone {
			stats.skipped++
			continue
		}
		byLayer[l] = append(byLayer[l], e)
		stats.ways[l]++
	}

	for _, l := range layerOrder {
		terrain := layerTerrain[l]
		for _, e := range byLayer[l] {
			if closedWay(e.Geometry) {
				stats.painted[l] += g.FillPolygons([]geojson.Polygon{{toRing(e.Geometry)}}, terrain)
				continue
			}
			stats.painted[l] += g.StrokeLines([]geojson.Line{toLine(e.Geometry)}, widthOf(g, e.Tags), terrain, false)
		}
	}
	return stats
}

// closedWay は始点と終点が一致する way（面として塗るもの）かを返す。
func closedWay(geom []coord) bool {
	if len(geom) < 4 {
		return false
	}
	first, last := geom[0], geom[len(geom)-1]
	return first.Lat == last.Lat && first.Lon == last.Lon
}

func toRing(geom []coord) geojson.Ring {
	ring := make(geojson.Ring, len(geom))
	for i, c := range geom {
		ring[i] = geojson.Point{Lon: c.Lon, Lat: c.Lat}
	}
	return ring
}

func toLine(geom []coord) geojson.Line {
	line := make(geojson.Line, len(geom))
	for i, c := range geom {
		line[i] = geojson.Point{Lon: c.Lon, Lat: c.Lat}
	}
	return line
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

func report(g *worldgrid.Grid, stats paintStats, b worldgrid.Bounds, out, preview string) {
	const degreeMeters = 111_320.0
	latMeters := g.LatSpan * degreeMeters
	lonMeters := g.LonSpan * degreeMeters * 0.81

	fmt.Printf("街のマップを生成しました\n")
	fmt.Printf("  範囲   : 北緯 %.4f〜%.4f, 東経 %.4f〜%.4f\n", b.MinLat, b.MaxLat, b.MinLon, b.MaxLon)
	fmt.Printf("  グリッド: %d × %d = %d マス（%d次メッシュ）\n", g.Cols, g.Rows, g.Cols*g.Rows, g.Level)
	fmt.Printf("  1マス  : 南北 %.1fm × 東西 %.1fm\n", latMeters, lonMeters)
	fmt.Printf("  実寸   : 東西 %.2fkm × 南北 %.2fkm\n",
		float64(g.Cols)*lonMeters/1000, float64(g.Rows)*latMeters/1000)

	fmt.Printf("\n描いたもの:\n")
	for _, l := range layerOrder {
		fmt.Printf("  %-4s %6d本 -> %7d マス\n", layerNames[l], stats.ways[l], stats.painted[l])
	}
	fmt.Printf("  対象外 %5d本\n", stats.skipped)

	counts := g.Histogram()
	total := g.Cols * g.Rows
	fmt.Printf("\n地形の内訳:\n")
	for t := worldgrid.Terrain(0); t < worldgrid.TerrainCount; t++ {
		if counts[t] == 0 {
			continue
		}
		share := float64(counts[t]) / float64(total) * 100
		fmt.Printf("  %-4s %7d マス (%5.1f%%) %s\n", t, counts[t], share, strings.Repeat("#", int(share/2)))
	}
	var walkable int
	for t := worldgrid.Terrain(0); t < worldgrid.TerrainCount; t++ {
		if t.Walkable() {
			walkable += counts[t]
		}
	}
	fmt.Printf("\n  歩ける面積 %.1f%%\n", float64(walkable)/float64(total)*100)

	if info, err := os.Stat(out); err == nil {
		fmt.Printf("\n  %s (%.1f KB)\n", out, float64(info.Size())/1024)
	}
	if preview != "" {
		fmt.Printf("  %s\n", preview)
	}
	fmt.Printf("\n出典: OpenStreetMap contributors (ODbL)\n")
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

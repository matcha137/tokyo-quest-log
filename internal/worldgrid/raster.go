package worldgrid

import (
	"math"
	"sort"

	"tokyo-quest-log/internal/geojson"
)

// gridPos は経度緯度をグリッドの連続座標に変換する。
// x は西から東、y は北から南へ増える。マス (row, col) は
// x ∈ [col, col+1), y ∈ [row, row+1) を占め、中心は +0.5 の位置。
func (g *Grid) gridPos(p geojson.Point) (x, y float64) {
	x = (p.Lon - g.OriginLon) / g.LonSpan
	y = float64(g.Rows) - (p.Lat-g.OriginLat)/g.LatSpan
	return x, y
}

// ringPath は輪をグリッド座標へ写したもの。
type ringPath []struct{ x, y float64 }

func (g *Grid) toRingPath(ring geojson.Ring) ringPath {
	path := make(ringPath, len(ring))
	for i, p := range ring {
		path[i].x, path[i].y = g.gridPos(p)
	}
	return path
}

// FillPolygons は面の内側を terrain で塗り、塗ったマス数を返す。
// 穴（Polygon の2番目以降の輪）は偶奇規則で自動的に抜ける。
func (g *Grid) FillPolygons(polys []geojson.Polygon, terrain Terrain) int {
	painted := 0
	for _, poly := range polys {
		g.scanPolygon(poly, func(row, col int) {
			if g.At(row, col) != terrain {
				g.Set(row, col, terrain)
				painted++
			}
		})
	}
	return painted
}

// MaskOutside は面の外側にあるマスを terrain で塗る。
// 舞台の範囲を都域に限定する用途を想定している。
func (g *Grid) MaskOutside(polys []geojson.Polygon, terrain Terrain) int {
	inside := make([]bool, g.Cols*g.Rows)
	for _, poly := range polys {
		g.scanPolygon(poly, func(row, col int) {
			inside[row*g.Cols+col] = true
		})
	}
	painted := 0
	for i, in := range inside {
		if !in && g.Tiles[i] != terrain {
			g.Tiles[i] = terrain
			painted++
		}
	}
	return painted
}

// scanPolygon は面の内側のマスを走査する。各行の中心を通る水平線と
// 全ての辺との交点を求め、偶奇規則で内側の区間を塗る。
func (g *Grid) scanPolygon(poly geojson.Polygon, visit func(row, col int)) {
	paths := make([]ringPath, 0, len(poly))
	minY, maxY := math.Inf(1), math.Inf(-1)
	for _, ring := range poly {
		if len(ring) < 3 {
			continue
		}
		path := g.toRingPath(ring)
		for _, pt := range path {
			minY, maxY = math.Min(minY, pt.y), math.Max(maxY, pt.y)
		}
		paths = append(paths, path)
	}
	if len(paths) == 0 {
		return
	}

	firstRow := max(0, int(math.Floor(minY)))
	lastRow := min(g.Rows-1, int(math.Ceil(maxY)))
	var crossings []float64

	for row := firstRow; row <= lastRow; row++ {
		yc := float64(row) + 0.5
		crossings = crossings[:0]
		for _, path := range paths {
			for i := range path {
				a, b := path[i], path[(i+1)%len(path)]
				// 半開区間で判定し、頂点を二重に数えないようにする。
				if (a.y <= yc) == (b.y <= yc) {
					continue
				}
				crossings = append(crossings, a.x+(yc-a.y)/(b.y-a.y)*(b.x-a.x))
			}
		}
		if len(crossings) < 2 {
			continue
		}
		sort.Float64s(crossings)
		for i := 0; i+1 < len(crossings); i += 2 {
			// マス中心 (col+0.5) が区間に入るものを塗る。
			first := max(0, int(math.Ceil(crossings[i]-0.5)))
			last := min(g.Cols-1, int(math.Floor(crossings[i+1]-0.5)))
			for col := first; col <= last; col++ {
				visit(row, col)
			}
		}
	}
}

// StrokeLines は線が通るマスを terrain で塗り、塗ったマス数を返す。
// widthCells は線幅（マス単位、1以上）。onlyLand が true のとき、
// 海と都域外のマスは塗らない。河川が海上へはみ出すのを防ぐ。
func (g *Grid) StrokeLines(lines []geojson.Line, widthCells int, terrain Terrain, onlyLand bool) int {
	if widthCells < 1 {
		widthCells = 1
	}
	radius := float64(widthCells-1) / 2
	painted := 0

	paint := func(row, col int) {
		if row < 0 || row >= g.Rows || col < 0 || col >= g.Cols {
			return
		}
		current := g.At(row, col)
		if current == terrain {
			return
		}
		if onlyLand && (current == Sea || current == OutOfArea) {
			return
		}
		g.Set(row, col, terrain)
		painted++
	}

	for _, line := range lines {
		for i := 0; i+1 < len(line); i++ {
			x0, y0 := g.gridPos(line[i])
			x1, y1 := g.gridPos(line[i+1])
			dx, dy := x1-x0, y1-y0
			// マスを跳び越さないよう、半マス以下の間隔で標本化する。
			steps := int(math.Ceil(math.Hypot(dx, dy)/0.4)) + 1
			for s := 0; s <= steps; s++ {
				t := float64(s) / float64(steps)
				stamp(x0+dx*t, y0+dy*t, radius, paint)
			}
		}
	}
	return painted
}

// stamp は座標を中心に半径 radius マスの範囲を塗る。
func stamp(x, y, radius float64, paint func(row, col int)) {
	if radius <= 0 {
		paint(int(math.Floor(y)), int(math.Floor(x)))
		return
	}
	first := int(math.Floor(-radius))
	last := int(math.Ceil(radius))
	centerCol, centerRow := math.Floor(x), math.Floor(y)
	for dy := first; dy <= last; dy++ {
		for dx := first; dx <= last; dx++ {
			if math.Hypot(float64(dx), float64(dy)) > radius+0.5 {
				continue
			}
			paint(int(centerRow)+dy, int(centerCol)+dx)
		}
	}
}

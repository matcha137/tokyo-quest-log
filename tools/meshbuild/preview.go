package main

import (
	"image"
	"image/color"
	"image/png"
	"os"

	"tokyo-quest-log/internal/landmark"
	"tokyo-quest-log/internal/worldgrid"
)

// markColors はランドマーク種別ごとのプレビュー上の色。
// 地形の配色と衝突しない明度・彩度にして、小さくても見つけられるようにする。
var markColors = map[landmark.Kind]color.RGBA{
	landmark.KindCity:     {R: 240, G: 90, B: 70, A: 255},
	landmark.KindTown:     {R: 244, G: 160, B: 74, A: 255},
	landmark.KindPeak:     {R: 255, G: 255, B: 255, A: 255},
	landmark.KindDungeon:  {R: 168, G: 106, B: 214, A: 255},
	landmark.KindPort:     {R: 96, G: 214, B: 224, A: 255},
	landmark.KindShrine:   {R: 236, G: 108, B: 168, A: 255},
	landmark.KindSight:    {R: 118, G: 200, B: 126, A: 255},
	landmark.KindFacility: {R: 232, G: 224, B: 110, A: 255},
}

// writePreviewPNG はグリッドを1タイル scale ピクセルで画像化する。
// ランドマークがあれば印を重ねる。
func writePreviewPNG(g *worldgrid.Grid, marks []landmark.Placed, path string, scale int) error {
	if scale < 1 {
		scale = 1
	}
	img := image.NewRGBA(image.Rect(0, 0, g.Cols*scale, g.Rows*scale))
	for row := 0; row < g.Rows; row++ {
		for col := 0; col < g.Cols; col++ {
			fillCell(img, row, col, scale, g.At(row, col).Color())
		}
	}
	for _, m := range marks {
		drawMark(img, m, scale)
	}

	f, err := os.Create(path)
	if err != nil {
		return err
	}
	defer f.Close()
	return png.Encode(f, img)
}

func fillCell(img *image.RGBA, row, col, scale int, c color.RGBA) {
	for dy := 0; dy < scale; dy++ {
		for dx := 0; dx < scale; dx++ {
			img.SetRGBA(col*scale+dx, row*scale+dy, c)
		}
	}
}

// drawMark はランドマークを、縮尺が小さくても視認できる大きさの点で描く。
func drawMark(img *image.RGBA, m landmark.Placed, scale int) {
	c, ok := markColors[m.Kind]
	if !ok {
		c = color.RGBA{R: 255, G: 255, B: 255, A: 255}
	}
	size := max(scale*2, 4)
	centerX := m.Col*scale + scale/2
	centerY := m.Row*scale + scale/2
	bounds := img.Bounds()
	for dy := -size / 2; dy <= size/2; dy++ {
		for dx := -size / 2; dx <= size/2; dx++ {
			x, y := centerX+dx, centerY+dy
			if x < 0 || y < 0 || x >= bounds.Dx() || y >= bounds.Dy() {
				continue
			}
			// 縁を暗くして、明るい地形の上でも輪郭が残るようにする。
			if abs(dx) == size/2 || abs(dy) == size/2 {
				img.SetRGBA(x, y, color.RGBA{R: 20, G: 24, B: 28, A: 255})
				continue
			}
			img.SetRGBA(x, y, c)
		}
	}
}

func abs(v int) int {
	if v < 0 {
		return -v
	}
	return v
}

package main

import (
	"image"
	"image/color"
	"image/png"
	"os"

	"tokyo-quest-log/internal/worldgrid"
)

// writePreviewPNG はグリッドを1タイル scale ピクセルで画像化する。
func writePreviewPNG(g *worldgrid.Grid, path string, scale int) error {
	if scale < 1 {
		scale = 1
	}
	img := image.NewRGBA(image.Rect(0, 0, g.Cols*scale, g.Rows*scale))
	for row := 0; row < g.Rows; row++ {
		for col := 0; col < g.Cols; col++ {
			fillCell(img, row, col, scale, g.At(row, col).Color())
		}
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

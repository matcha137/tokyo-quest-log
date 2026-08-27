package main

import (
	"image"
	"image/color"
	"image/png"
	"os"

	"tokyo-quest-log/internal/worldgrid"
)

// palette は確認用プレビューの配色。実際のゲーム内タイル絵とは無関係で、
// 生成結果が東京の形になっているかを目視するためだけに使う。
var palette = [worldgrid.TerrainCount]color.RGBA{
	worldgrid.Sea:          {R: 27, G: 74, B: 107, A: 255},
	worldgrid.Lowland:      {R: 112, G: 156, B: 112, A: 255},
	worldgrid.Plain:        {R: 138, G: 176, B: 96, A: 255},
	worldgrid.Plateau:      {R: 168, G: 174, B: 104, A: 255},
	worldgrid.Hill:         {R: 154, G: 140, B: 84, A: 255},
	worldgrid.Mountain:     {R: 124, G: 106, B: 74, A: 255},
	worldgrid.HighMountain: {R: 198, G: 198, B: 190, A: 255},
	worldgrid.Water:        {R: 74, G: 138, B: 176, A: 255},
	worldgrid.OutOfArea:    {R: 38, G: 44, B: 50, A: 255},
}

// writePreviewPNG はグリッドを1タイル scale ピクセルで画像化する。
func writePreviewPNG(g *worldgrid.Grid, path string, scale int) error {
	if scale < 1 {
		scale = 1
	}
	img := image.NewRGBA(image.Rect(0, 0, g.Cols*scale, g.Rows*scale))
	for row := 0; row < g.Rows; row++ {
		for col := 0; col < g.Cols; col++ {
			c := palette[g.At(row, col)]
			for dy := 0; dy < scale; dy++ {
				for dx := 0; dx < scale; dx++ {
					img.SetRGBA(col*scale+dx, row*scale+dy, c)
				}
			}
		}
	}
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	defer f.Close()
	return png.Encode(f, img)
}

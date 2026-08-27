package worldgrid

import "image/color"

// Palette は地形ごとの表示色。プレビュー画像、Tiledのタイルセット、
// ゲーム内描画で同じ色を使うため、ここを唯一の定義とする。
var Palette = [TerrainCount]color.RGBA{
	Sea:          {R: 27, G: 74, B: 107, A: 255},
	Lowland:      {R: 112, G: 156, B: 112, A: 255},
	Plain:        {R: 138, G: 176, B: 96, A: 255},
	Plateau:      {R: 168, G: 174, B: 104, A: 255},
	Hill:         {R: 154, G: 140, B: 84, A: 255},
	Mountain:     {R: 124, G: 106, B: 74, A: 255},
	HighMountain: {R: 198, G: 198, B: 190, A: 255},
	Water:        {R: 74, G: 138, B: 176, A: 255},
	OutOfArea:    {R: 38, G: 44, B: 50, A: 255},
}

// Color は地形の表示色を返す。未知の値は都域外の色にする。
func (t Terrain) Color() color.RGBA {
	if t >= TerrainCount {
		return Palette[OutOfArea]
	}
	return Palette[t]
}

package main

import (
	"fmt"
	"strings"

	"tokyo-quest-log/internal/worldgrid"
)

// band は標高の上限（未満）と、そこに割り当てる地形。
type band struct {
	maxElevation float64
	terrain      worldgrid.Terrain
}

// defaultBands は東京都の標高分布に合わせた区分。
//
// 東京都は東端の海抜0m地帯から西端の雲取山（2,017m）まで揃うため、
// 標高だけで古典的なJRPGの地形帯がそのまま得られる。
// どの区分にも入らない標高は HighMountain とする。
var defaultBands = []band{
	{5, worldgrid.Lowland},    // 江東5区などの海抜0m地帯・沖積低地
	{50, worldgrid.Plain},     // 沖積平野
	{150, worldgrid.Plateau},  // 武蔵野台地
	{400, worldgrid.Hill},     // 多摩丘陵
	{900, worldgrid.Mountain}, // 関東山地の縁辺（高尾山599mはここ）
}

func classify(elevation float64) worldgrid.Terrain {
	for _, b := range defaultBands {
		if elevation < b.maxElevation {
			return b.terrain
		}
	}
	return worldgrid.HighMountain
}

// describeBands は適用した区分を人間が読める形で返す。
func describeBands() string {
	var sb strings.Builder
	prev := "―"
	for _, b := range defaultBands {
		fmt.Fprintf(&sb, "  %-4s %s〜%.0fm\n", b.terrain, prev, b.maxElevation)
		prev = fmt.Sprintf("%.0f", b.maxElevation)
	}
	fmt.Fprintf(&sb, "  %-4s %sm以上\n", worldgrid.HighMountain, prev)
	fmt.Fprintf(&sb, "  %-4s 標高データなし（陸域外＝海・都域外）\n", worldgrid.Sea)
	return sb.String()
}

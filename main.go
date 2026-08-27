package main

import (
	"flag"
	"fmt"
	"log"
	"os"

	"github.com/hajimehoshi/ebiten/v2"
)

func main() {
	mode := flag.String("mode", "world", "起動する画面 world（ワールドマップ）または quest（初期の探索デモ）")
	worldPath := flag.String("world", "assets/world.bin", "ワールドマップデータのパス")
	flag.Parse()

	game, title, err := newGame(*mode, *worldPath)
	if err != nil {
		log.Fatal(err)
	}

	ebiten.SetWindowSize(screenWidth, screenHeight)
	ebiten.SetWindowTitle(title)
	ebiten.SetWindowResizingMode(ebiten.WindowResizingModeEnabled)
	if err := ebiten.RunGame(game); err != nil {
		log.Fatal(err)
	}
}

func newGame(mode, worldPath string) (ebiten.Game, string, error) {
	switch mode {
	case "world":
		game, err := NewWorldGame(worldPath)
		return game, "TOKYO QUEST LOG — 東京ワールドマップ", err
	case "quest":
		game, err := NewGame()
		return game, "TOKYO QUEST LOG — 街を知る探索RPG", err
	}
	flag.Usage()
	fmt.Fprintln(os.Stderr)
	return nil, "", fmt.Errorf("未知の -mode: %q（world または quest）", mode)
}

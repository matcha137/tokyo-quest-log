package main

import (
	"os"
	"path/filepath"
	"testing"

	"tokyo-quest-log/internal/worldgrid"
)

// writeTestWorld は小さなワールドマップデータをファイルに書き出す。
func writeTestWorld(t *testing.T) string {
	t.Helper()
	g, err := worldgrid.New(worldgrid.TokyoMainland, 5)
	if err != nil {
		t.Fatal(err)
	}
	for i := range g.Tiles {
		g.Tiles[i] = worldgrid.Plain
	}
	path := filepath.Join(t.TempDir(), "world.bin")
	f, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	if _, err := g.WriteTo(f); err != nil {
		t.Fatal(err)
	}
	return path
}

// 同梱のランドマークが埋め込まれ、ワールドマップに配置されること。
func TestNewWorldGameLoadsEmbeddedLandmarks(t *testing.T) {
	g, err := NewWorldGame(writeTestWorld(t))
	if err != nil {
		t.Fatal(err)
	}
	if g.loadErr != nil {
		t.Fatalf("ワールドマップを読み込めない: %v", g.loadErr)
	}
	if g.world == nil {
		t.Fatal("探索状態が作られていない")
	}
	if len(g.world.Placed) < 20 {
		t.Errorf("配置されたランドマーク = %d 件, 20件以上を期待", len(g.world.Placed))
	}
	// 最初のランドマークの上に立っていること。
	if g.world.LandmarkHere() == nil {
		t.Error("ランドマークの上から開始していない")
	}
}

// データが無くても起動でき、案内画面へ落ちること。
func TestNewWorldGameSurvivesMissingData(t *testing.T) {
	g, err := NewWorldGame(filepath.Join(t.TempDir(), "存在しない.bin"))
	if err != nil {
		t.Fatalf("データが無いだけで起動に失敗した: %v", err)
	}
	if g.loadErr == nil {
		t.Error("読み込みエラーが記録されていない")
	}
	if err := g.Update(); err != nil {
		t.Errorf("案内画面で Update が失敗した: %v", err)
	}
}

// 壊れたデータでも起動が落ちないこと。
func TestNewWorldGameRejectsGarbage(t *testing.T) {
	path := filepath.Join(t.TempDir(), "broken.bin")
	if err := os.WriteFile(path, []byte("NOT A WORLD MAP"), 0o644); err != nil {
		t.Fatal(err)
	}
	g, err := NewWorldGame(path)
	if err != nil {
		t.Fatal(err)
	}
	if g.loadErr == nil {
		t.Error("壊れたデータが受理された")
	}
}

func TestUpdateWithoutInput(t *testing.T) {
	g, err := NewWorldGame(writeTestWorld(t))
	if err != nil {
		t.Fatal(err)
	}
	before := g.world.Player
	for i := 0; i < 10; i++ {
		if err := g.Update(); err != nil {
			t.Fatal(err)
		}
	}
	if g.world.Player != before {
		t.Errorf("入力が無いのに動いた: %+v -> %+v", before, g.world.Player)
	}
}

func TestNewGameModes(t *testing.T) {
	worldPath := writeTestWorld(t)
	if _, title, err := newGame("world", worldPath); err != nil || title == "" {
		t.Errorf("world モードが作れない: %v", err)
	}
	if _, title, err := newGame("quest", worldPath); err != nil || title == "" {
		t.Errorf("quest モードが作れない: %v", err)
	}
	if _, _, err := newGame("unknown", worldPath); err == nil {
		t.Error("未知のモードがエラーにならなかった")
	}
}

func TestViewSizeCoversScreen(t *testing.T) {
	cols, rows := viewSize()
	if cols*worldTileSize < screenWidth {
		t.Errorf("横方向が画面を覆えていない: %v マス", cols)
	}
	if rows*worldTileSize < screenHeight-hudHeight {
		t.Errorf("縦方向が地図領域を覆えていない: %v マス", rows)
	}
}

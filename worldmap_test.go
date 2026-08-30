package main

import (
	"os"
	"path/filepath"
	"testing"

	"tokyo-quest-log/internal/landmark"
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

// 同梱の assets/world.bin が都域に限定されていること。
// 都域外のマスが存在し、ランドマークが全て到達可能な地形に載っている必要がある。
func TestBundledWorldIsClippedToTokyo(t *testing.T) {
	g, err := NewWorldGame("assets/world.bin")
	if err != nil {
		t.Fatal(err)
	}
	if g.loadErr != nil {
		t.Fatalf("同梱のワールドマップを読み込めない: %v", g.loadErr)
	}

	counts := g.world.Grid.Histogram()
	total := g.world.Grid.Cols * g.world.Grid.Rows
	outside := counts[worldgrid.OutOfArea]
	if outside == 0 {
		t.Fatal("都域外のマスが無い。境界が適用されていない")
	}
	// 外接矩形に対して都域はおよそ4割。極端にずれていれば境界の適用ミスを疑う。
	if share := float64(outside) / float64(total); share < 0.4 || share > 0.75 {
		t.Errorf("都域外の割合 = %.1f%%, 40〜75%%を期待", share*100)
	}

	for _, p := range g.world.Placed {
		if terrain := g.world.Grid.At(p.Row, p.Col); !terrain.Walkable() {
			t.Errorf("%s が通行できないマスに載っている: %s", p.Name, terrain)
		}
	}
}

// 都域外へは踏み出せないこと。実データの端で確かめる。
func TestCannotWalkOutOfTokyo(t *testing.T) {
	g, err := NewWorldGame("assets/world.bin")
	if err != nil || g.loadErr != nil {
		t.Fatalf("同梱のワールドマップを読み込めない: %v %v", err, g.loadErr)
	}
	w := g.world

	// 都域外のマスを1つ探し、そこへ入れないことを確かめる。
	found := false
	for row := 0; row < w.Grid.Rows && !found; row++ {
		for col := 0; col < w.Grid.Cols; col++ {
			if w.Grid.At(row, col) == worldgrid.OutOfArea {
				if w.CanEnter(row, col) {
					t.Fatalf("都域外 (%d,%d) に入れてしまう", row, col)
				}
				found = true
				break
			}
		}
	}
	if !found {
		t.Fatal("都域外のマスが見つからない")
	}
}

// 拠点と観光スポットの両方が地図に載ること。
func TestBundledLandmarksIncludeTourism(t *testing.T) {
	g, err := NewWorldGame("assets/world.bin")
	if err != nil || g.loadErr != nil {
		t.Fatalf("同梱のワールドマップを読み込めない: %v %v", err, g.loadErr)
	}

	kinds := map[landmark.Kind]int{}
	for _, p := range g.world.Placed {
		kinds[p.Kind]++
	}
	if kinds[landmark.KindCity] == 0 {
		t.Error("拠点となる町が載っていない")
	}
	if kinds[landmark.KindSight] == 0 {
		t.Error("観光スポットが載っていない")
	}
	if len(g.world.Placed) < 45 {
		t.Errorf("ランドマーク総数 = %d, 45件以上を期待", len(g.world.Placed))
	}

	// 拠点の東京は観光スポットに押し出されず残っていること。
	found := false
	for _, p := range g.world.Placed {
		if p.ID == "tokyo" {
			found = true
		}
	}
	if !found {
		t.Error("拠点の東京が失われている")
	}
}

// 同梱の街マップが読め、街として成立していること。
func TestBundledTownMap(t *testing.T) {
	g, err := NewWorldGame("assets/town_tokyo.bin")
	if err != nil {
		t.Fatal(err)
	}
	if g.loadErr != nil {
		t.Fatalf("街マップを読み込めない: %v", g.loadErr)
	}
	if g.world.Grid.Level != 10 {
		t.Errorf("メッシュ次数 = %d, want 10", g.world.Grid.Level)
	}

	counts := g.world.Grid.Histogram()
	for _, terrain := range []worldgrid.Terrain{worldgrid.Road, worldgrid.Building, worldgrid.Rail, worldgrid.Park} {
		if counts[terrain] == 0 {
			t.Errorf("%s が1マスも無い", terrain)
		}
	}

	// 街として歩ける余地があること。建物で埋まっていたら遊べない。
	total := g.world.Grid.Cols * g.world.Grid.Rows
	var walkable int
	for terrain := worldgrid.Terrain(0); terrain < worldgrid.TerrainCount; terrain++ {
		if terrain.Walkable() {
			walkable += counts[terrain]
		}
	}
	if share := float64(walkable) / float64(total); share < 0.4 || share > 0.9 {
		t.Errorf("歩ける割合 = %.1f%%, 40〜90%%を期待", share*100)
	}

	// 開始位置が壁の中でないこと。
	row, col := g.world.Cell()
	if !g.world.Grid.At(row, col).Walkable() {
		t.Errorf("開始位置 (%d,%d) が %s で動けない", row, col, g.world.Grid.At(row, col))
	}
}

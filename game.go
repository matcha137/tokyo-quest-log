package main

import (
	"bytes"
	"fmt"
	"image/color"
	"math"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/hajimehoshi/ebiten/v2/examples/resources/fonts"
	"github.com/hajimehoshi/ebiten/v2/inpututil"
	"github.com/hajimehoshi/ebiten/v2/text/v2"
	"github.com/hajimehoshi/ebiten/v2/vector"
)

const (
	screenWidth  = 1280
	screenHeight = 720
	mapX         = 28
	mapY         = 76
	mapW         = 842
	mapH         = 614
	panelX       = 894
)

var (
	ink      = color.RGBA{R: 237, G: 238, B: 228, A: 255}
	muted    = color.RGBA{R: 157, G: 169, B: 168, A: 255}
	bg       = color.RGBA{R: 15, G: 25, B: 33, A: 255}
	mapBase  = color.RGBA{R: 28, G: 43, B: 50, A: 255}
	panel    = color.RGBA{R: 22, G: 35, B: 43, A: 255}
	line     = color.RGBA{R: 65, G: 84, B: 88, A: 255}
	movement = color.RGBA{R: 238, G: 183, B: 83, A: 255}
	life     = color.RGBA{R: 92, G: 194, B: 142, A: 255}
	terrain  = color.RGBA{R: 78, G: 155, B: 192, A: 255}
	accent   = color.RGBA{R: 245, G: 113, B: 91, A: 255}
	fog      = color.RGBA{R: 67, G: 80, B: 85, A: 205}
	fogLight = color.RGBA{R: 83, G: 96, B: 99, A: 115}
)

type Game struct {
	world      *World
	fontSource *text.GoTextFaceSource
	ticks      int
	lastUnlock Layer
	toast      string
	toastTicks int
}

func NewGame() (*Game, error) {
	source, err := text.NewGoTextFaceSource(bytes.NewReader(fonts.MPlus1pRegular_ttf))
	if err != nil {
		return nil, fmt.Errorf("日本語フォントを読み込む: %w", err)
	}
	return &Game{world: NewWorld(), fontSource: source}, nil
}

func (g *Game) Update() error {
	g.ticks++
	if g.toastTicks > 0 {
		g.toastTicks--
	}

	if g.world.ShowComplete {
		if inpututil.IsKeyJustPressed(ebiten.KeyR) {
			g.world.Reset()
			g.toast = "新しい探索を開始しました"
			g.toastTicks = 150
		}
		return nil
	}

	if g.world.ActiveQuest >= 0 {
		if inpututil.IsKeyJustPressed(ebiten.KeyEscape) {
			g.world.ActiveQuest = -1
			g.world.Feedback = ""
			return nil
		}
		for i, key := range []ebiten.Key{ebiten.Key1, ebiten.Key2, ebiten.Key3} {
			if inpututil.IsKeyJustPressed(key) {
				layer := g.world.Quests[g.world.ActiveQuest].Layer
				if g.world.Answer(i) {
					g.lastUnlock = layer
					g.toast = layerName(layer) + "を復元しました"
					g.toastTicks = 210
				}
				return nil
			}
		}
		return nil
	}

	if inpututil.IsKeyJustPressed(ebiten.KeyR) {
		g.world.Reset()
		g.toast = "探索記録をリセットしました"
		g.toastTicks = 150
		return nil
	}

	const speed = 3.2
	dx, dy := 0.0, 0.0
	if ebiten.IsKeyPressed(ebiten.KeyArrowLeft) || ebiten.IsKeyPressed(ebiten.KeyA) {
		dx--
	}
	if ebiten.IsKeyPressed(ebiten.KeyArrowRight) || ebiten.IsKeyPressed(ebiten.KeyD) {
		dx++
	}
	if ebiten.IsKeyPressed(ebiten.KeyArrowUp) || ebiten.IsKeyPressed(ebiten.KeyW) {
		dy--
	}
	if ebiten.IsKeyPressed(ebiten.KeyArrowDown) || ebiten.IsKeyPressed(ebiten.KeyS) {
		dy++
	}
	if dx != 0 || dy != 0 {
		length := math.Hypot(dx, dy)
		g.world.Player.X += dx / length * speed
		g.world.Player.Y += dy / length * speed
		g.world.Player.X = clamp(g.world.Player.X, 18, mapW-18)
		g.world.Player.Y = clamp(g.world.Player.Y, 18, mapH-18)
	}

	if inpututil.IsKeyJustPressed(ebiten.KeySpace) || inpututil.IsKeyJustPressed(ebiten.KeyEnter) {
		if !g.world.OpenNearbyQuest(52) {
			g.toast = "イベントの印に近づいてください"
			g.toastTicks = 90
		}
	}
	return nil
}

func (g *Game) Draw(screen *ebiten.Image) {
	screen.Fill(bg)
	g.drawHeader(screen)
	g.drawMap(screen)
	g.drawPanel(screen)
	if g.toastTicks > 0 && !g.world.ShowComplete {
		g.drawToast(screen)
	}
	if g.world.ActiveQuest >= 0 {
		g.drawQuestModal(screen)
	}
	if g.world.ShowComplete {
		g.drawCompleteModal(screen)
	}
}

func (g *Game) Layout(_, _ int) (int, int) {
	return screenWidth, screenHeight
}

func (g *Game) drawHeader(dst *ebiten.Image) {
	g.drawText(dst, "TOKYO QUEST LOG", 28, 25, 24, ink)
	g.drawText(dst, "街を知ると、地図がひらく。", 260, 29, 17, muted)
	progress := g.world.CompletedCount()
	g.drawText(dst, fmt.Sprintf("MAP  %d / %d", progress, len(g.world.Quests)), 1060, 28, 17, ink)
	vector.FillRect(dst, 1142, 31, 105, 8, line, false)
	if progress > 0 {
		vector.FillRect(dst, 1142, 31, float32(105*progress/len(g.world.Quests)), 8, movement, false)
	}
}

func (g *Game) drawMap(dst *ebiten.Image) {
	vector.FillRect(dst, mapX, mapY, mapW, mapH, mapBase, false)
	vector.StrokeRect(dst, mapX, mapY, mapW, mapH, 1, line, false)

	// 未探索時にも座標の手掛かりだけは残す。
	for x := 35; x < mapW; x += 70 {
		vector.StrokeLine(dst, mapX+float32(x), mapY, mapX+float32(x), mapY+mapH, 1, color.RGBA{R: 51, G: 66, B: 70, A: 100}, false)
	}
	for y := 35; y < mapH; y += 70 {
		vector.StrokeLine(dst, mapX, mapY+float32(y), mapX+mapW, mapY+float32(y), 1, color.RGBA{R: 51, G: 66, B: 70, A: 100}, false)
	}

	for _, feature := range g.world.Features {
		if !g.world.LayerUnlocked(feature.Layer) {
			continue
		}
		g.drawFeature(dst, feature)
	}

	g.drawFog(dst)
	g.drawQuests(dst)
	g.drawPlayer(dst)

	near := g.world.NearbyQuest(52)
	if near >= 0 {
		q := g.world.Quests[near]
		x := float32(mapX + int(q.Position.X) - 92)
		y := float32(mapY + int(q.Position.Y) - 58)
		vector.FillRect(dst, x, y, 184, 35, color.RGBA{R: 12, G: 20, B: 25, A: 235}, false)
		g.drawText(dst, "SPACE  調べる", float64(x+27), float64(y+8), 15, ink)
	}

	g.drawText(dst, "WASD / 矢印  移動     SPACE  調べる     R  リセット", mapX+18, mapY+mapH-31, 14, muted)
}

func (g *Game) drawFeature(dst *ebiten.Image, f MapFeature) {
	c := layerColor(f.Layer)
	switch f.Kind {
	case "road":
		for i := 1; i < len(f.Points); i++ {
			a, b := g.mapPoint(f.Points[i-1]), g.mapPoint(f.Points[i])
			vector.StrokeLine(dst, a.X, a.Y, b.X, b.Y, 8, color.RGBA{R: 50, G: 59, B: 58, A: 255}, true)
			vector.StrokeLine(dst, a.X, a.Y, b.X, b.Y, 3, c, true)
		}
	case "water", "contour":
		var path vector.Path
		for i, p := range f.Points {
			mp := g.mapPoint(p)
			if i == 0 {
				path.MoveTo(mp.X, mp.Y)
			} else {
				path.LineTo(mp.X, mp.Y)
			}
		}
		path.Close()
		drawOp := &vector.DrawPathOptions{AntiAlias: true}
		if f.Kind == "water" {
			drawOp.ColorScale.ScaleWithColor(color.RGBA{R: terrain.R, G: terrain.G, B: terrain.B, A: 115})
			vector.FillPath(dst, &path, nil, drawOp)
		} else {
			drawOp.ColorScale.ScaleWithColor(color.RGBA{R: 105, G: 132, B: 105, A: 70})
			vector.FillPath(dst, &path, nil, drawOp)
			strokeOp := &vector.StrokeOptions{Width: 2}
			lineOp := &vector.DrawPathOptions{AntiAlias: true}
			lineOp.ColorScale.ScaleWithColor(terrain)
			vector.StrokePath(dst, &path, strokeOp, lineOp)
		}
	case "slope":
		for i := 1; i < len(f.Points); i++ {
			a, b := g.mapPoint(f.Points[i-1]), g.mapPoint(f.Points[i])
			vector.StrokeLine(dst, a.X, a.Y, b.X, b.Y, 5, terrain, true)
		}
	default:
		p := g.mapPoint(f.Points[0])
		vector.FillCircle(dst, p.X, p.Y, 13, color.RGBA{R: 18, G: 31, B: 35, A: 255}, true)
		vector.StrokeCircle(dst, p.X, p.Y, 13, 3, c, true)
		g.drawFeatureIcon(dst, f.Kind, p.X, p.Y, c)
	}

	if f.Kind != "road" && len(f.Points) > 0 {
		p := g.mapPoint(f.Points[0])
		g.drawText(dst, f.Label, float64(p.X+18), float64(p.Y-9), 13, ink)
	}
}

func (g *Game) drawFeatureIcon(dst *ebiten.Image, kind string, x, y float32, c color.RGBA) {
	switch kind {
	case "station":
		vector.FillRect(dst, x-5, y-6, 10, 12, c, false)
		vector.FillRect(dst, x-8, y+5, 16, 2, c, false)
	case "library":
		vector.FillRect(dst, x-7, y-6, 6, 12, c, false)
		vector.FillRect(dst, x+1, y-6, 6, 12, c, false)
	case "park":
		vector.FillCircle(dst, x, y-3, 7, c, true)
		vector.FillRect(dst, x-2, y+2, 4, 7, c, false)
	case "museum":
		vector.FillRect(dst, x-8, y+4, 16, 3, c, false)
		vector.FillRect(dst, x-7, y-4, 3, 8, c, false)
		vector.FillRect(dst, x-1, y-4, 3, 8, c, false)
		vector.FillRect(dst, x+5, y-4, 3, 8, c, false)
	case "rest":
		vector.FillRect(dst, x-8, y, 16, 3, c, false)
		vector.FillRect(dst, x-6, y+2, 2, 6, c, false)
		vector.FillRect(dst, x+4, y+2, 2, 6, c, false)
	}
}

func (g *Game) drawFog(dst *ebiten.Image) {
	completed := g.world.CompletedCount()
	if completed == len(g.world.Quests) {
		return
	}
	alpha := uint8(120 - completed*25)
	for y := 0; y < 7; y++ {
		for x := 0; x < 10; x++ {
			if (x+y+completed)%3 == 0 {
				cx := float32(mapX + 25 + x*86)
				cy := float32(mapY + 35 + y*84)
				vector.FillCircle(dst, cx, cy, 49, color.RGBA{R: fog.R, G: fog.G, B: fog.B, A: alpha}, true)
				vector.FillCircle(dst, cx+28, cy+7, 33, color.RGBA{R: fogLight.R, G: fogLight.G, B: fogLight.B, A: alpha / 2}, true)
			}
		}
	}
}

func (g *Game) drawQuests(dst *ebiten.Image) {
	for i, q := range g.world.Quests {
		p := g.mapPoint(q.Position)
		if q.Completed {
			vector.FillCircle(dst, p.X, p.Y, 17, color.RGBA{R: 24, G: 45, B: 42, A: 255}, true)
			vector.StrokeCircle(dst, p.X, p.Y, 17, 3, layerColor(q.Layer), true)
			vector.StrokeLine(dst, p.X-7, p.Y, p.X-1, p.Y+6, 3, ink, true)
			vector.StrokeLine(dst, p.X-1, p.Y+6, p.X+9, p.Y-7, 3, ink, true)
			continue
		}
		pulse := float32(3 * math.Sin(float64(g.ticks+i*35)/15))
		vector.FillCircle(dst, p.X, p.Y, 18+pulse, color.RGBA{R: accent.R, G: accent.G, B: accent.B, A: 45}, true)
		vector.FillCircle(dst, p.X, p.Y, 10, accent, true)
		g.drawText(dst, "!", float64(p.X-3.5), float64(p.Y-10), 17, bg)
	}
}

func (g *Game) drawPlayer(dst *ebiten.Image) {
	p := g.mapPoint(g.world.Player)
	vector.FillCircle(dst, p.X+2, p.Y+3, 15, color.RGBA{R: 0, G: 0, B: 0, A: 90}, true)
	vector.FillCircle(dst, p.X, p.Y, 12, ink, true)
	vector.StrokeCircle(dst, p.X, p.Y, 12, 3, accent, true)
	vector.FillCircle(dst, p.X, p.Y-2, 4, accent, true)
}

func (g *Game) drawPanel(dst *ebiten.Image) {
	vector.FillRect(dst, panelX, mapY, 358, mapH, panel, false)
	vector.StrokeRect(dst, panelX, mapY, 358, mapH, 1, line, false)
	g.drawText(dst, "探索ノート", panelX+22, mapY+20, 22, ink)
	g.drawText(dst, "イベントを解き、街の意味を地図に残そう。", panelX+22, mapY+52, 14, muted)

	y := float64(mapY + 89)
	for i, q := range g.world.Quests {
		cardColor := color.RGBA{R: 29, G: 45, B: 53, A: 255}
		if q.Completed {
			cardColor = color.RGBA{R: 27, G: 52, B: 48, A: 255}
		}
		vector.FillRect(dst, panelX+16, float32(y), 326, 112, cardColor, false)
		vector.FillRect(dst, panelX+16, float32(y), 5, 112, layerColor(q.Layer), false)
		status := fmt.Sprintf("0%d", i+1)
		if q.Completed {
			status = "✓"
		}
		g.drawText(dst, status, panelX+35, y+17, 18, layerColor(q.Layer))
		g.drawText(dst, q.ShortTitle, panelX+75, y+16, 17, ink)
		g.drawText(dst, layerName(q.Layer), panelX+75, y+45, 13, muted)
		state := "未発見 — マップ上の ! を探す"
		if q.Completed {
			state = "復元済み — " + shortSource(q.Source)
		}
		g.drawText(dst, state, panelX+35, y+77, 12, muted)
		y += 126
	}

	vector.StrokeLine(dst, panelX+22, 557, 1229, 557, 1, line, false)
	g.drawText(dst, "このMVPで確かめること", panelX+22, 575, 15, ink)
	g.drawText(dst, "・イベントで地図への理解が深まるか", panelX+22, 605, 13, muted)
	g.drawText(dst, "・完成した地図が外出意欲につながるか", panelX+22, 630, 13, muted)
	g.drawText(dst, "※ 現在は説明用サンプルデータ", panelX+22, 666, 12, color.RGBA{R: 219, G: 161, B: 92, A: 255})
}

func (g *Game) drawQuestModal(dst *ebiten.Image) {
	q := g.world.Quests[g.world.ActiveQuest]
	vector.FillRect(dst, 0, 0, screenWidth, screenHeight, color.RGBA{R: 5, G: 10, B: 13, A: 210}, false)
	x, y, w, h := float32(225), float32(100), float32(830), float32(520)
	vector.FillRect(dst, x, y, w, h, panel, false)
	vector.StrokeRect(dst, x, y, w, h, 2, layerColor(q.Layer), false)
	vector.FillRect(dst, x, y, 9, h, layerColor(q.Layer), false)

	g.drawText(dst, layerName(q.Layer), 270, 135, 15, layerColor(q.Layer))
	g.drawText(dst, q.Title, 270, 169, 28, ink)
	for i, lineText := range q.Description {
		g.drawText(dst, lineText, 270, float64(221+i*28), 16, muted)
	}
	vector.StrokeLine(dst, 270, 290, 1007, 290, 1, line, false)
	g.drawText(dst, q.Question, 270, 322, 18, ink)

	for i, choice := range q.Choices {
		cy := float32(367 + i*58)
		vector.FillRect(dst, 270, cy, 737, 43, color.RGBA{R: 31, G: 49, B: 57, A: 255}, false)
		vector.StrokeRect(dst, 270, cy, 737, 43, 1, line, false)
		g.drawText(dst, choice, 291, float64(cy+10), 16, ink)
	}
	if qSourceY := float64(556); g.world.Feedback != "" {
		g.drawText(dst, g.world.Feedback, 270, 545, 14, accent)
	} else {
		g.drawText(dst, q.Source, 270, qSourceY, 13, muted)
	}
	g.drawText(dst, "1 / 2 / 3 で回答     ESC で閉じる", 711, 584, 13, muted)
}

func (g *Game) drawCompleteModal(dst *ebiten.Image) {
	vector.FillRect(dst, 0, 0, screenWidth, screenHeight, color.RGBA{R: 6, G: 12, B: 15, A: 225}, false)
	x, y, w, h := float32(175), float32(76), float32(930), float32(570)
	vector.FillRect(dst, x, y, w, h, panel, false)
	vector.StrokeRect(dst, x, y, w, h, 2, movement, false)

	g.drawText(dst, "MAP COMPLETE", 480, 118, 16, movement)
	g.drawText(dst, "あなたの街の地図が完成した", 366, 157, 31, ink)
	g.drawText(dst, "道を覚えただけではない。街で過ごす場所と、地形の記憶がつながった。", 286, 210, 16, muted)

	labels := []struct {
		title string
		body  string
		c     color.RGBA
	}{
		{"移動", "駅と道から\n街のつながりを発見", movement},
		{"生活", "公共施設から\n休日の居場所を発見", life},
		{"地形", "池と高低差から\n街の成り立ちを発見", terrain},
	}
	for i, item := range labels {
		cx := float32(273 + i*250)
		vector.FillRect(dst, cx, 270, 216, 150, color.RGBA{R: 29, G: 46, B: 53, A: 255}, false)
		vector.FillRect(dst, cx, 270, 216, 6, item.c, false)
		g.drawText(dst, item.title, float64(cx+77), 297, 20, item.c)
		for j, lineText := range splitLines(item.body) {
			g.drawText(dst, lineText, float64(cx+27), float64(341+j*27), 15, ink)
		}
	}

	vector.StrokeLine(dst, 255, 458, 1025, 458, 1, line, false)
	g.drawText(dst, "次の一歩", 255, 483, 15, accent)
	g.drawText(dst, "実データ版では、この地図から現実の街歩きルートをつくります。", 255, 514, 16, ink)
	g.drawText(dst, "プレイ前後で『街への理解』と『行ってみたい気持ち』を測定します。", 255, 542, 15, muted)
	g.drawText(dst, "R  もう一度探索する", 522, 594, 14, movement)
}

func (g *Game) drawToast(dst *ebiten.Image) {
	w := float32(360)
	x := float32(mapX) + (mapW-w)/2
	y := float32(mapY + 22)
	vector.FillRect(dst, x, y, w, 46, color.RGBA{R: 12, G: 24, B: 28, A: 245}, false)
	vector.StrokeRect(dst, x, y, w, 46, 1, layerColor(g.lastUnlock), false)
	g.drawText(dst, g.toast, float64(x+26), float64(y+12), 15, ink)
}

func (g *Game) drawText(dst *ebiten.Image, value string, x, y, size float64, clr color.Color) {
	op := &text.DrawOptions{}
	op.GeoM.Translate(x, y)
	op.ColorScale.ScaleWithColor(clr)
	op.LineSpacing = size * 1.45
	text.Draw(dst, value, &text.GoTextFace{Source: g.fontSource, Size: size}, op)
}

func (g *Game) mapPoint(p Point) struct{ X, Y float32 } {
	return struct{ X, Y float32 }{X: float32(mapX) + float32(p.X), Y: float32(mapY) + float32(p.Y)}
}

func layerColor(layer Layer) color.RGBA {
	switch layer {
	case LayerMovement:
		return movement
	case LayerLife:
		return life
	case LayerTerrain:
		return terrain
	default:
		return muted
	}
}

func layerName(layer Layer) string {
	switch layer {
	case LayerMovement:
		return "移動レイヤー"
	case LayerLife:
		return "生活レイヤー"
	case LayerTerrain:
		return "地形レイヤー"
	default:
		return "未分類"
	}
}

func shortSource(source string) string {
	switch source {
	case "公共交通・道路オープンデータを想定":
		return "交通・道路"
	case "公共施設・公園オープンデータを想定":
		return "施設・公園"
	case "標高・河川オープンデータを想定":
		return "標高・河川"
	default:
		return source
	}
}

func splitLines(s string) []string {
	lines := []string{}
	start := 0
	for i, r := range s {
		if r == '\n' {
			lines = append(lines, s[start:i])
			start = i + 1
		}
	}
	lines = append(lines, s[start:])
	return lines
}

func clamp(value, minValue, maxValue float64) float64 {
	if value < minValue {
		return minValue
	}
	if value > maxValue {
		return maxValue
	}
	return value
}

package main

import (
	"bytes"
	_ "embed"
	"fmt"
	"image/color"
	"math"
	"os"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/hajimehoshi/ebiten/v2/examples/resources/fonts"
	"github.com/hajimehoshi/ebiten/v2/inpututil"
	"github.com/hajimehoshi/ebiten/v2/text/v2"
	"github.com/hajimehoshi/ebiten/v2/vector"

	"tokyo-quest-log/internal/landmark"
	"tokyo-quest-log/internal/worldgrid"
	"tokyo-quest-log/internal/worldsim"
)

//go:embed assets/landmarks.json
var landmarkJSON []byte

const (
	worldTileSize = 20
	hudHeight     = 116
	// 1秒あたりに進むマス数。1マス約250mなので、徒歩でも都内を
	// 1分強で横断できる圧縮率にしている。ワールドマップとしての速さを優先。
	walkTilesPerSecond = 5.0
	runTilesPerSecond  = 11.0
)

// WorldGame はワールドマップの探索画面。
type WorldGame struct {
	world      *worldsim.World
	fontSource *text.GoTextFaceSource
	tiles      [worldgrid.TerrainCount]*ebiten.Image

	loadErr   error
	worldPath string

	message      string
	messageTicks int
	dialog       *landmark.Placed
}

// NewWorldGame はワールドマップ画面を作る。マップデータが無い場合でも
// 起動し、生成手順を案内する画面を出す。
func NewWorldGame(worldPath string) (*WorldGame, error) {
	source, err := text.NewGoTextFaceSource(bytes.NewReader(fonts.MPlus1pRegular_ttf))
	if err != nil {
		return nil, fmt.Errorf("日本語フォントを読み込む: %w", err)
	}
	g := &WorldGame{fontSource: source, worldPath: worldPath}
	for t := worldgrid.Terrain(0); t < worldgrid.TerrainCount; t++ {
		img := ebiten.NewImage(worldTileSize, worldTileSize)
		img.Fill(t.Color())
		g.tiles[t] = img
	}
	if err := g.loadWorld(worldPath); err != nil {
		g.loadErr = err
	}
	return g, nil
}

func (g *WorldGame) loadWorld(path string) error {
	f, err := os.Open(path)
	if err != nil {
		return err
	}
	defer f.Close()
	grid, err := worldgrid.ReadFrom(f)
	if err != nil {
		return fmt.Errorf("%s: %w", path, err)
	}
	marks, err := landmark.Load(bytes.NewReader(landmarkJSON))
	if err != nil {
		return err
	}
	placed, _ := landmark.Place(grid, marks)
	g.world = worldsim.New(grid, placed, 1)
	return nil
}

func (g *WorldGame) Layout(_, _ int) (int, int) { return screenWidth, screenHeight }

func (g *WorldGame) Update() error {
	if g.messageTicks > 0 {
		g.messageTicks--
	}
	if g.loadErr != nil {
		return nil
	}
	if g.dialog != nil {
		if inpututil.IsKeyJustPressed(ebiten.KeyEscape) || inpututil.IsKeyJustPressed(ebiten.KeySpace) ||
			inpututil.IsKeyJustPressed(ebiten.KeyEnter) {
			g.dialog = nil
		}
		return nil
	}

	g.handleBoarding()
	g.handleEnter()
	g.handleMovement()
	return nil
}

func (g *WorldGame) handleMovement() {
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
	if dx == 0 && dy == 0 {
		return
	}

	speed := walkTilesPerSecond
	if ebiten.IsKeyPressed(ebiten.KeyShiftLeft) || ebiten.IsKeyPressed(ebiten.KeyShiftRight) {
		speed = runTilesPerSecond
	}
	result := g.world.Step(dx, dy, speed/60)

	if result.Arrived != nil {
		g.notify(result.Arrived.Name + " に着いた")
	}
	if result.Encounter {
		g.notify("何かが近づいてくる気配がする（" + g.world.Tile().String() + "）")
	}
}

// handleBoarding は港での乗り降りを扱う。海へ出るには港が要る。
func (g *WorldGame) handleBoarding() {
	if !inpututil.IsKeyJustPressed(ebiten.KeyB) {
		return
	}
	here := g.world.LandmarkHere()
	if here == nil || here.Kind != landmark.KindPort {
		g.notify("船に乗り降りできるのは港だけだ")
		return
	}
	if g.world.Vehicle == worldsim.ByShip {
		g.world.Vehicle = worldsim.OnFoot
		g.notify(here.Name + " で船を降りた")
		return
	}
	g.world.Vehicle = worldsim.ByShip
	g.notify(here.Name + " から船に乗った")
}

func (g *WorldGame) handleEnter() {
	if !inpututil.IsKeyJustPressed(ebiten.KeySpace) && !inpututil.IsKeyJustPressed(ebiten.KeyEnter) {
		return
	}
	if here := g.world.LandmarkHere(); here != nil {
		g.dialog = here
		return
	}
	if near := g.world.NearestLandmark(3); near != nil {
		g.notify(near.Name + " はすぐ近くだ")
		return
	}
	g.notify("あたりには何もない")
}

func (g *WorldGame) notify(message string) {
	g.message = message
	g.messageTicks = 180
}

func (g *WorldGame) Draw(screen *ebiten.Image) {
	screen.Fill(bg)
	if g.loadErr != nil {
		g.drawMissingData(screen)
		return
	}
	g.drawTiles(screen)
	g.drawLandmarks(screen)
	g.drawPlayer(screen)
	g.drawHUD(screen)
	if g.dialog != nil {
		g.drawDialog(screen)
	}
	if g.messageTicks > 0 {
		g.drawMessage(screen)
	}
}

// viewSize は地形を描く領域のマス数を返す。
func viewSize() (cols, rows float64) {
	return float64(screenWidth) / worldTileSize, float64(screenHeight-hudHeight) / worldTileSize
}

func (g *WorldGame) drawTiles(dst *ebiten.Image) {
	viewCols, viewRows := viewSize()
	cam := g.world.Camera(viewCols, viewRows)
	startCol, startRow := int(math.Floor(cam.X)), int(math.Floor(cam.Y))
	offsetX := (cam.X - float64(startCol)) * worldTileSize
	offsetY := (cam.Y - float64(startRow)) * worldTileSize

	// 画面に映る範囲だけ描く。全域を1枚の画像に持つと数百MBになるため、
	// 毎フレームの間引き描画で済ませる。
	for row := 0; row <= int(viewRows)+1; row++ {
		for col := 0; col <= int(viewCols)+1; col++ {
			terrain := g.world.Grid.At(startRow+row, startCol+col)
			op := &ebiten.DrawImageOptions{}
			op.GeoM.Translate(float64(col)*worldTileSize-offsetX, float64(row)*worldTileSize-offsetY)
			dst.DrawImage(g.tiles[terrain], op)
		}
	}
}

// screenPos はマス座標を画面座標に変換する。
func (g *WorldGame) screenPos(x, y float64) (float32, float32) {
	viewCols, viewRows := viewSize()
	cam := g.world.Camera(viewCols, viewRows)
	return float32((x - cam.X) * worldTileSize), float32((y - cam.Y) * worldTileSize)
}

func landmarkColor(kind landmark.Kind) color.RGBA {
	switch kind {
	case landmark.KindCity:
		return color.RGBA{R: 240, G: 90, B: 70, A: 255}
	case landmark.KindTown:
		return color.RGBA{R: 244, G: 160, B: 74, A: 255}
	case landmark.KindPeak:
		return color.RGBA{R: 245, G: 245, B: 245, A: 255}
	case landmark.KindDungeon:
		return color.RGBA{R: 168, G: 106, B: 214, A: 255}
	case landmark.KindPort:
		return color.RGBA{R: 96, G: 214, B: 224, A: 255}
	case landmark.KindShrine:
		return color.RGBA{R: 236, G: 108, B: 168, A: 255}
	}
	return color.RGBA{R: 232, G: 224, B: 110, A: 255}
}

func (g *WorldGame) drawLandmarks(dst *ebiten.Image) {
	mapBottom := float32(screenHeight - hudHeight)
	for i := range g.world.Placed {
		p := &g.world.Placed[i]
		x, y := g.screenPos(float64(p.Col)+0.5, float64(p.Row)+0.5)
		if x < -40 || y < -40 || x > screenWidth+40 || y > mapBottom+40 {
			continue
		}
		const size = 13
		vector.FillRect(dst, x-size/2, y-size/2, size, size, landmarkColor(p.Kind), false)
		vector.StrokeRect(dst, x-size/2, y-size/2, size, size, 1.5, color.RGBA{R: 18, G: 22, B: 26, A: 255}, false)
		if y < mapBottom-14 {
			g.drawText(dst, p.Name, float64(x)+10, float64(y)-7, 12, ink)
		}
	}
}

func (g *WorldGame) drawPlayer(dst *ebiten.Image) {
	x, y := g.screenPos(g.world.Player.X, g.world.Player.Y)
	vector.FillCircle(dst, x, y, 7, color.RGBA{R: 20, G: 24, B: 28, A: 255}, true)
	vector.FillCircle(dst, x, y, 5, accent, true)
	if g.world.Vehicle == worldsim.ByShip {
		vector.StrokeCircle(dst, x, y, 10, 2, color.RGBA{R: 96, G: 214, B: 224, A: 255}, true)
	}
}

func (g *WorldGame) drawHUD(dst *ebiten.Image) {
	top := float32(screenHeight - hudHeight)
	vector.FillRect(dst, 0, top, screenWidth, hudHeight, panel, false)
	vector.StrokeLine(dst, 0, top, screenWidth, top, 1, line, false)

	w := g.world
	pos := w.LatLon()
	row, col := w.Cell()

	g.drawText(dst, "TOKYO QUEST LOG — ワールドマップ", 24, float64(top)+14, 16, ink)
	g.drawText(dst, fmt.Sprintf("現在地  %s / %s", w.Tile(), w.Vehicle), 24, float64(top)+42, 14, muted)
	g.drawText(dst, fmt.Sprintf("北緯 %.4f  東経 %.4f  （%d行 %d列）", pos.Lat, pos.Lon, row, col), 24, float64(top)+64, 14, muted)
	g.drawText(dst, fmt.Sprintf("踏破距離  %.1f km", w.DistanceMeters()/1000), 24, float64(top)+86, 14, muted)

	if here := w.LandmarkHere(); here != nil {
		g.drawText(dst, here.Name, 470, float64(top)+42, 16, landmarkColor(here.Kind))
		g.drawText(dst, "SPACE  調べる", 470, float64(top)+66, 13, muted)
	} else if near := w.NearestLandmark(6); near != nil {
		g.drawText(dst, near.Name+" が近い", 470, float64(top)+42, 14, muted)
	}

	g.drawText(dst, "WASD / 矢印  移動      SHIFT  走る      SPACE  調べる      B  乗船／下船",
		screenWidth-560, float64(top)+86, 13, muted)
}

func (g *WorldGame) drawDialog(dst *ebiten.Image) {
	const w, h = 520.0, 190.0
	x, y := (screenWidth-w)/2, (screenHeight-hudHeight-h)/2
	vector.FillRect(dst, float32(x), float32(y), w, h, color.RGBA{R: 12, G: 20, B: 25, A: 240}, false)
	vector.StrokeRect(dst, float32(x), float32(y), w, h, 1.5, line, false)

	p := g.dialog
	g.drawText(dst, p.Name, x+28, y+26, 22, landmarkColor(p.Kind))
	g.drawText(dst, kindLabel(p.Kind), x+28, y+62, 14, muted)
	if p.Note != "" {
		g.drawText(dst, p.Note, x+28, y+88, 14, ink)
	}
	if code, err := p.MeshCode(); err == nil {
		g.drawText(dst, fmt.Sprintf("北緯 %.4f  東経 %.4f", p.Lat, p.Lon), x+28, y+118, 13, muted)
		g.drawText(dst, "5次メッシュ "+code, x+28, y+138, 13, muted)
	}
	g.drawText(dst, "SPACE / ESC  閉じる", x+28, y+h-30, 13, muted)
}

func kindLabel(kind landmark.Kind) string {
	switch kind {
	case landmark.KindCity:
		return "大きな町"
	case landmark.KindTown:
		return "町"
	case landmark.KindPeak:
		return "山頂"
	case landmark.KindDungeon:
		return "秘境"
	case landmark.KindPort:
		return "港"
	case landmark.KindShrine:
		return "社寺"
	}
	return "施設"
}

func (g *WorldGame) drawMessage(dst *ebiten.Image) {
	const w, h = 620.0, 40.0
	x := float32((screenWidth - w) / 2)
	y := float32(screenHeight - hudHeight - h - 18)
	vector.FillRect(dst, x, y, w, h, color.RGBA{R: 12, G: 20, B: 25, A: 225}, false)
	vector.StrokeRect(dst, x, y, w, h, 1, line, false)
	g.drawText(dst, g.message, float64(x)+20, float64(y)+11, 15, ink)
}

// drawMissingData はワールドマップのデータが無いときの案内を出す。
func (g *WorldGame) drawMissingData(dst *ebiten.Image) {
	lines := []string{
		"ワールドマップのデータが見つかりません。",
		"",
		"探した場所: " + g.worldPath,
		"",
		"標高メッシュから生成してください:",
		"  go run ./tools/meshbuild -in <標高CSV> -out " + g.worldPath,
		"",
		"境界と河川を重ねる場合:",
		"  -boundary <都域GeoJSON> -overlay water=<河川GeoJSON>",
		"",
		"詳しくは tools/meshbuild/README.md を参照してください。",
	}
	g.drawText(dst, "データがありません", 80, 96, 26, accent)
	for i, l := range lines {
		g.drawText(dst, l, 80, 160+float64(i)*28, 15, ink)
	}
	g.drawText(dst, fmt.Sprintf("(%v)", g.loadErr), 80, 160+float64(len(lines))*28+16, 12, muted)
}

func (g *WorldGame) drawText(dst *ebiten.Image, value string, x, y, size float64, clr color.Color) {
	op := &text.DrawOptions{}
	op.GeoM.Translate(x, y)
	op.ColorScale.ScaleWithColor(clr)
	op.LineSpacing = size * 1.45
	text.Draw(dst, value, &text.GoTextFace{Source: g.fontSource, Size: size}, op)
}

// demtiles は国土地理院の標高タイル（PNG形式）を取得し、
// 標準地域メッシュごとの平均標高としてCSVに書き出す。
//
//	go run ./tools/demtiles -z 11 -out data/elevation.csv
//
// 出力したCSVは meshbuild の -in にそのまま渡せる。
//
// 出典: 国土地理院 標高タイル（https://maps.gsi.go.jp/development/ichiran.html）
// 利用にあたっては国土地理院コンテンツ利用規約に従い、出典を明記すること。
package main

import (
	"bufio"
	"bytes"
	"flag"
	"fmt"
	"image"
	"image/png"
	"io"
	"math"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"time"

	"tokyo-quest-log/internal/geomesh"
	"tokyo-quest-log/internal/worldgrid"
)

const (
	tileURLTemplate = "https://cyberjapandata.gsi.go.jp/xyz/dem_png/%d/%d/%d.png"
	tileSize        = 256
	// noDataValue は標高タイルが無効値に使う画素値。
	noDataValue = 1 << 23
	// elevationUnit は画素値から標高（m）への換算係数。
	elevationUnit = 0.01
	userAgent     = "tokyo-quest-log/dev (world map generation)"
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, "demtiles:", err)
		os.Exit(1)
	}
}

func run() error {
	opts, err := parseFlags()
	if err != nil {
		return err
	}

	minX, maxX := lonToTileX(opts.bounds.MinLon, opts.zoom), lonToTileX(opts.bounds.MaxLon, opts.zoom)
	minY, maxY := latToTileY(opts.bounds.MaxLat, opts.zoom), latToTileY(opts.bounds.MinLat, opts.zoom)
	total := (maxX - minX + 1) * (maxY - minY + 1)

	fmt.Printf("国土地理院 標高タイルを取得します\n")
	fmt.Printf("  ズーム   : %d（1画素 約%.0fm）\n", opts.zoom, pixelMeters(opts.zoom, opts.bounds.MinLat))
	fmt.Printf("  タイル   : x %d..%d, y %d..%d = %d 枚\n", minX, maxX, minY, maxY, total)
	fmt.Printf("  取得先   : cyberjapandata.gsi.go.jp\n")
	fmt.Printf("  保存先   : %s\n\n", opts.cacheDir)

	acc := newAccumulator()
	fetched, cached := 0, 0
	for y := minY; y <= maxY; y++ {
		for x := minX; x <= maxX; x++ {
			img, fromCache, err := loadTile(opts, x, y)
			if err != nil {
				return err
			}
			if fromCache {
				cached++
			} else {
				fetched++
			}
			acc.addTile(img, x, y, opts)
			fmt.Printf("\r  進捗: %d / %d 枚", fetched+cached, total)
		}
	}
	fmt.Printf("\n  取得 %d 枚、キャッシュ利用 %d 枚\n\n", fetched, cached)

	if acc.count() == 0 {
		return fmt.Errorf("有効な標高が1件も得られませんでした")
	}
	if err := acc.writeCSV(opts.out); err != nil {
		return err
	}
	acc.report(opts)
	return nil
}

type options struct {
	zoom     int
	level    int
	bounds   worldgrid.Bounds
	out      string
	cacheDir string
	client   *http.Client
}

func parseFlags() (options, error) {
	var (
		zoom     = flag.Int("z", 11, "取得するズームレベル（11なら約62m/画素）")
		level    = flag.Int("level", 5, "集約先のメッシュ次数（5=約250m）")
		out      = flag.String("out", "data/elevation.csv", "出力するCSVのパス")
		cacheDir = flag.String("cache", "data/demtiles", "取得したタイルを保存する場所")
	)
	flag.Parse()

	if *zoom < 1 || *zoom > 14 {
		return options{}, fmt.Errorf("ズームは1〜14の範囲: %d", *zoom)
	}
	return options{
		zoom: *zoom, level: *level, bounds: worldgrid.TokyoMainland,
		out: *out, cacheDir: *cacheDir,
		client: &http.Client{Timeout: 30 * time.Second},
	}, nil
}

// loadTile はタイルを取得する。既に保存済みならそれを使い、
// 同じタイルを何度も国土地理院へ取りに行かないようにする。
func loadTile(opts options, x, y int) (image.Image, bool, error) {
	path := filepath.Join(opts.cacheDir, fmt.Sprintf("%d", opts.zoom), fmt.Sprintf("%d_%d.png", x, y))
	if f, err := os.Open(path); err == nil {
		defer f.Close()
		img, err := png.Decode(f)
		if err == nil {
			return img, true, nil
		}
		// 壊れたキャッシュは取り直す。
	}

	url := fmt.Sprintf(tileURLTemplate, opts.zoom, x, y)
	req, err := http.NewRequest(http.MethodGet, url, nil)
	if err != nil {
		return nil, false, err
	}
	req.Header.Set("User-Agent", userAgent)
	resp, err := opts.client.Do(req)
	if err != nil {
		return nil, false, fmt.Errorf("%s: %w", url, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, false, fmt.Errorf("%s: HTTP %d", url, resp.StatusCode)
	}
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, false, err
	}

	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return nil, false, err
	}
	if err := os.WriteFile(path, body, 0o644); err != nil {
		return nil, false, err
	}
	img, err := png.Decode(bytes.NewReader(body))
	if err != nil {
		return nil, false, fmt.Errorf("%s: PNGを読めない: %w", url, err)
	}
	return img, false, nil
}

// accumulator はメッシュごとに標高を平均する。
// タイルの画素は約62mでメッシュは約250mなので、1マスに複数画素が入る。
type accumulator struct {
	sum   map[string]float64
	hits  map[string]int
	minEl float64
	maxEl float64
}

func newAccumulator() *accumulator {
	return &accumulator{
		sum: map[string]float64{}, hits: map[string]int{},
		minEl: math.Inf(1), maxEl: math.Inf(-1),
	}
}

func (a *accumulator) count() int { return len(a.sum) }

func (a *accumulator) addTile(img image.Image, tileX, tileY int, opts options) {
	worldPixels := float64(tileSize) * math.Exp2(float64(opts.zoom))
	for py := 0; py < tileSize; py++ {
		globalY := float64(tileY*tileSize+py) + 0.5
		lat := pixelYToLat(globalY, worldPixels)
		if lat < opts.bounds.MinLat || lat > opts.bounds.MaxLat {
			continue
		}
		for px := 0; px < tileSize; px++ {
			globalX := float64(tileX*tileSize+px) + 0.5
			lon := pixelXToLon(globalX, worldPixels)
			if lon < opts.bounds.MinLon || lon > opts.bounds.MaxLon {
				continue
			}
			elevation, ok := decodeElevation(img, px, py)
			if !ok {
				continue // 無効値。海や範囲外
			}
			code, err := geomesh.Encode(geomesh.LatLon{Lat: lat, Lon: lon}, opts.level)
			if err != nil {
				continue
			}
			a.sum[code] += elevation
			a.hits[code]++
			a.minEl = math.Min(a.minEl, elevation)
			a.maxEl = math.Max(a.maxEl, elevation)
		}
	}
}

// decodeElevation は標高タイルの画素値を標高(m)に変換する。
// 仕様: x = 2^16 R + 2^8 G + B。x が 2^23 なら無効値、
// 2^23 未満はそのまま、超える場合は 2^24 を引いて負の標高とする。
func decodeElevation(img image.Image, px, py int) (float64, bool) {
	bounds := img.Bounds()
	r, g, b, _ := img.At(bounds.Min.X+px, bounds.Min.Y+py).RGBA()
	// RGBA() は16bitで返るため8bitへ落とす。
	value := int(r>>8)<<16 | int(g>>8)<<8 | int(b>>8)
	if value == noDataValue {
		return 0, false
	}
	if value > noDataValue {
		value -= 1 << 24
	}
	return float64(value) * elevationUnit, true
}

func (a *accumulator) writeCSV(path string) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	codes := make([]string, 0, len(a.sum))
	for code := range a.sum {
		codes = append(codes, code)
	}
	sort.Strings(codes)

	f, err := os.Create(path)
	if err != nil {
		return err
	}
	defer f.Close()
	w := bufio.NewWriter(f)
	fmt.Fprintln(w, "meshcode,elevation")
	for _, code := range codes {
		fmt.Fprintf(w, "%s,%.1f\n", code, a.sum[code]/float64(a.hits[code]))
	}
	return w.Flush()
}

func (a *accumulator) report(opts options) {
	var totalHits int
	for _, n := range a.hits {
		totalHits += n
	}
	fmt.Printf("メッシュへ集約しました\n")
	fmt.Printf("  メッシュ : %d 件（%d次メッシュ）\n", len(a.sum), opts.level)
	fmt.Printf("  画素     : %d 個（1メッシュあたり平均 %.1f 画素）\n", totalHits, float64(totalHits)/float64(len(a.sum)))
	fmt.Printf("  標高範囲 : %.1fm 〜 %.1fm\n", a.minEl, a.maxEl)
	if info, err := os.Stat(opts.out); err == nil {
		fmt.Printf("  %s (%.1f MB)\n", opts.out, float64(info.Size())/(1024*1024))
	}
	fmt.Printf("\n出典: 国土地理院 標高タイル\n")

	// 標高が妥当かを既知の地点で確かめる。桁がずれていればここで気づける。
	fmt.Printf("\n既知の地点との照合:\n")
	for _, spot := range []struct {
		name     string
		point    geomesh.LatLon
		expected string
	}{
		{"雲取山", geomesh.LatLon{Lat: 35.8553, Lon: 138.9436}, "約2,017m"},
		{"高尾山", geomesh.LatLon{Lat: 35.6253, Lon: 139.2436}, "約599m"},
		{"東京駅", geomesh.LatLon{Lat: 35.6812, Lon: 139.7671}, "数m"},
	} {
		code, err := geomesh.Encode(spot.point, opts.level)
		if err != nil {
			continue
		}
		if hits := a.hits[code]; hits > 0 {
			fmt.Printf("  %-6s %7.1fm （実際は%s）\n", spot.name, a.sum[code]/float64(hits), spot.expected)
		} else {
			fmt.Printf("  %-6s データなし\n", spot.name)
		}
	}
}

// Web メルカトルの画素座標と緯度経度の相互変換。

func lonToTileX(lon float64, zoom int) int {
	return int(math.Floor((lon + 180) / 360 * math.Exp2(float64(zoom))))
}

func latToTileY(lat float64, zoom int) int {
	r := lat * math.Pi / 180
	return int(math.Floor((1 - math.Log(math.Tan(r)+1/math.Cos(r))/math.Pi) / 2 * math.Exp2(float64(zoom))))
}

func pixelXToLon(px, worldPixels float64) float64 {
	return px/worldPixels*360 - 180
}

func pixelYToLat(py, worldPixels float64) float64 {
	n := math.Pi * (1 - 2*py/worldPixels)
	return math.Atan(math.Sinh(n)) * 180 / math.Pi
}

func pixelMeters(zoom int, lat float64) float64 {
	return 40075016.686 / (tileSize * math.Exp2(float64(zoom))) * math.Cos(lat*math.Pi/180)
}

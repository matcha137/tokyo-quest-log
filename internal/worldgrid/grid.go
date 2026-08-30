// Package worldgrid は東京都をタイル化したワールドマップのグリッドを表す。
//
// グリッドの1マスは標準地域メッシュの1区画に一致する。5次メッシュを使うと
// 1マス約232m（南北）×約282m（東西）になる。
package worldgrid

import (
	"encoding/binary"
	"fmt"
	"io"
	"math"
	"strings"

	"tokyo-quest-log/internal/geomesh"
)

// Terrain はワールドマップの地形種別。
type Terrain uint8

const (
	// Sea は標高データが存在しないマス。国土数値情報の標高メッシュは陸域のみを
	// 収録するため、欠損はそのまま海（および都域外）を意味する。
	Sea          Terrain = iota
	Lowland              // 低地・海抜0m地帯
	Plain                // 平地
	Plateau              // 台地
	Hill                 // 丘陵
	Mountain             // 山地
	HighMountain         // 高山

	// Water は河川・湖沼などの内水面。海とは分けて扱う。
	// 海は船で越える対象、内水面は橋や渡しで越える対象、という区別を想定する。
	Water
	// OutOfArea は都域外。標高データは存在するが舞台の外側であるマス。
	// 海として塗ると埼玉や神奈川が水没するため、専用の種別を設ける。
	OutOfArea

	// ここから下は街の中を歩く詳細マップ用。
	// 東京駅周辺の高低差は2km四方で15m程度しかなく、標高では街の構造が出ない。
	// 道路・建物・線路といった人工物の方が街の骨格なので、それを地形として扱う。
	Ground   // 街区の地面
	Road     // 道路
	Rail     // 線路
	Building // 建物
	Park     // 公園・緑地

	TerrainCount
)

var terrainNames = [TerrainCount]string{
	"海", "低地", "平地", "台地", "丘陵", "山地", "高山", "内水面", "都域外",
	"地面", "道路", "線路", "建物", "公園",
}

// terrainKeys はコマンドラインなどで地形を指定するための名前。
var terrainKeys = map[string]Terrain{
	"sea": Sea, "海": Sea,
	"lowland": Lowland, "低地": Lowland,
	"plain": Plain, "平地": Plain,
	"plateau": Plateau, "台地": Plateau,
	"hill": Hill, "丘陵": Hill,
	"mountain": Mountain, "山地": Mountain,
	"highmountain": HighMountain, "高山": HighMountain,
	"water": Water, "内水面": Water,
	"outofarea": OutOfArea, "都域外": OutOfArea,
	"ground": Ground, "地面": Ground,
	"road": Road, "道路": Road,
	"rail": Rail, "線路": Rail,
	"building": Building, "建物": Building,
	"park": Park, "公園": Park,
}

// TerrainFromName は名前から地形種別を引く。
func TerrainFromName(name string) (Terrain, error) {
	if t, ok := terrainKeys[strings.ToLower(strings.TrimSpace(name))]; ok {
		return t, nil
	}
	return 0, fmt.Errorf("未知の地形名: %q", name)
}

func (t Terrain) String() string {
	if t >= TerrainCount {
		return fmt.Sprintf("Terrain(%d)", uint8(t))
	}
	return terrainNames[t]
}

// Walkable は徒歩で進入できる地形かを返す。
// 通れないのは水域と舞台の外、それに街の建物と線路だけにする。
// 高山を塞ぐと奥多摩の山域がまるごと到達不能になり、雲取山のような
// 目的地が置けなくなるため、険しさは通行可否ではなく遭遇率で表す。
func (t Terrain) Walkable() bool {
	switch t {
	case Sea, Water, OutOfArea, Rail, Building:
		return false
	}
	return true
}

// Bounds は緯度経度の矩形範囲。
type Bounds struct {
	MinLat, MinLon float64
	MaxLat, MaxLon float64
}

// TokyoMainland は東京都本土（23区・多摩地域）を覆う範囲。
// 島嶼部は本土から遥かに離れるため、別マップとして切り出す。
var TokyoMainland = Bounds{MinLat: 35.49, MinLon: 138.92, MaxLat: 35.91, MaxLon: 139.93}

// Grid はタイル化されたワールドマップ。Tiles は行優先で、行0が北端。
type Grid struct {
	Level     int
	OriginLat float64 // 南西端の緯度（メッシュ境界にスナップ済み）
	OriginLon float64 // 南西端の経度
	LatSpan   float64 // 1マスの緯度幅（度）
	LonSpan   float64 // 1マスの経度幅（度）
	Cols      int
	Rows      int
	Tiles     []Terrain
}

// New は範囲を覆うグリッドを確保する。原点はメッシュ境界にスナップするため、
// 実際の範囲は引数よりわずかに広くなる。
func New(b Bounds, level int) (*Grid, error) {
	latSpan, lonSpan, err := Span(level)
	if err != nil {
		return nil, err
	}
	origin, err := geomesh.Encode(geomesh.LatLon{Lat: b.MinLat, Lon: b.MinLon}, level)
	if err != nil {
		return nil, fmt.Errorf("南西端をメッシュへ変換: %w", err)
	}
	cell, err := geomesh.Decode(origin)
	if err != nil {
		return nil, err
	}
	cols := int(math.Ceil((b.MaxLon - cell.SW.Lon) / lonSpan))
	rows := int(math.Ceil((b.MaxLat - cell.SW.Lat) / latSpan))
	if cols <= 0 || rows <= 0 {
		return nil, fmt.Errorf("範囲が不正: %+v", b)
	}
	return &Grid{
		Level:     level,
		OriginLat: cell.SW.Lat,
		OriginLon: cell.SW.Lon,
		LatSpan:   latSpan,
		LonSpan:   lonSpan,
		Cols:      cols,
		Rows:      rows,
		Tiles:     make([]Terrain, cols*rows),
	}, nil
}

// Span は geomesh.Span の薄いラッパ。呼び出し側の import を減らす。
func Span(level int) (latSpan, lonSpan float64, err error) {
	return geomesh.Span(level)
}

// CellCenter は指定マスの中心座標を返す。行0が北端であることに注意。
func (g *Grid) CellCenter(row, col int) geomesh.LatLon {
	return geomesh.LatLon{
		Lat: g.OriginLat + (float64(g.Rows-1-row)+0.5)*g.LatSpan,
		Lon: g.OriginLon + (float64(col)+0.5)*g.LonSpan,
	}
}

// At は指定マスの地形を返す。範囲外は Sea を返す。
func (g *Grid) At(row, col int) Terrain {
	if row < 0 || row >= g.Rows || col < 0 || col >= g.Cols {
		return Sea
	}
	return g.Tiles[row*g.Cols+col]
}

// Set は指定マスの地形を設定する。
func (g *Grid) Set(row, col int, t Terrain) {
	if row < 0 || row >= g.Rows || col < 0 || col >= g.Cols {
		return
	}
	g.Tiles[row*g.Cols+col] = t
}

// Histogram は地形ごとのマス数を返す。生成結果の妥当性確認に使う。
func (g *Grid) Histogram() [TerrainCount]int {
	var counts [TerrainCount]int
	for _, t := range g.Tiles {
		if t < TerrainCount {
			counts[t]++
		}
	}
	return counts
}

const (
	fileMagic   = "TQLG"
	fileVersion = 1
)

// WriteTo はグリッドをバイナリ形式で書き出す。
func (g *Grid) WriteTo(w io.Writer) (int64, error) {
	cw := &countingWriter{w: w}
	if _, err := cw.Write([]byte(fileMagic)); err != nil {
		return cw.n, err
	}
	header := []any{
		uint8(fileVersion), uint8(g.Level),
		uint32(g.Cols), uint32(g.Rows),
		g.OriginLat, g.OriginLon, g.LatSpan, g.LonSpan,
	}
	for _, field := range header {
		if err := binary.Write(cw, binary.BigEndian, field); err != nil {
			return cw.n, err
		}
	}
	tiles := make([]byte, len(g.Tiles))
	for i, t := range g.Tiles {
		tiles[i] = byte(t)
	}
	_, err := cw.Write(tiles)
	return cw.n, err
}

// ReadFrom はバイナリ形式のグリッドを読み込む。
func ReadFrom(r io.Reader) (*Grid, error) {
	magic := make([]byte, len(fileMagic))
	if _, err := io.ReadFull(r, magic); err != nil {
		return nil, fmt.Errorf("マジックの読み込み: %w", err)
	}
	if string(magic) != fileMagic {
		return nil, fmt.Errorf("ワールドマップの形式ではない: %q", magic)
	}
	var version, level uint8
	var cols, rows uint32
	g := &Grid{}
	fields := []any{&version, &level, &cols, &rows, &g.OriginLat, &g.OriginLon, &g.LatSpan, &g.LonSpan}
	for _, field := range fields {
		if err := binary.Read(r, binary.BigEndian, field); err != nil {
			return nil, fmt.Errorf("ヘッダの読み込み: %w", err)
		}
	}
	if version != fileVersion {
		return nil, fmt.Errorf("未対応のバージョン: %d", version)
	}
	g.Level, g.Cols, g.Rows = int(level), int(cols), int(rows)
	tiles := make([]byte, int(cols)*int(rows))
	if _, err := io.ReadFull(r, tiles); err != nil {
		return nil, fmt.Errorf("タイルの読み込み: %w", err)
	}
	g.Tiles = make([]Terrain, len(tiles))
	for i, b := range tiles {
		if Terrain(b) >= TerrainCount {
			return nil, fmt.Errorf("未知の地形コード %d (offset %d)", b, i)
		}
		g.Tiles[i] = Terrain(b)
	}
	return g, nil
}

type countingWriter struct {
	w io.Writer
	n int64
}

func (c *countingWriter) Write(p []byte) (int, error) {
	n, err := c.w.Write(p)
	c.n += int64(n)
	return n, err
}

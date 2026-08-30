// Package geomesh は日本の標準地域メッシュ（JIS X 0410）のメッシュコードと
// 緯度経度を相互変換する。
//
// 5次メッシュ（約250m四方）がそのままワールドマップのタイル1マスに対応するため、
// メッシュコードの格子構造をタイルグリッドの座標系として利用する。
//
// 6次以降は規格の範囲外で、本パッケージ独自の拡張である。4次・5次と同じく
// 直前の区画を2×2に分割し、南西=1 南東=2 北西=3 北東=4 の1桁を足していく。
// 街の中を歩くマップには250mでは粗すぎるため、同じ座標系のまま
// 細かい格子を得られるようにしている。
package geomesh

import (
	"fmt"
	"math"
	"strconv"
)

const (
	// StandardMaxLevel は規格が定める最大次数。5次 = 1/4地域メッシュ = 約250m。
	StandardMaxLevel = 5
	// MaxLevel は本パッケージが扱う最大次数。10次で東京付近およそ7m × 9m。
	MaxLevel = 10
)

// LatLon は十進度で表した緯度経度。
type LatLon struct {
	Lat float64
	Lon float64
}

// Span は指定次数のメッシュ1区画が占める緯度幅・経度幅を度で返す。
//
// 緯度幅と経度幅は等しくない。5次メッシュは緯度7.5秒・経度11.25秒で、
// 東京付近では約232m × 約282m となり、正方形ではない点に注意。
func Span(level int) (latSpan, lonSpan float64, err error) {
	switch {
	case level == 1:
		return 2.0 / 3.0, 1.0, nil // 40分 × 1度
	case level == 2:
		return 1.0 / 12.0, 1.0 / 8.0, nil // 5分 × 7分30秒
	case level == 3:
		return 1.0 / 120.0, 1.0 / 80.0, nil // 30秒 × 45秒
	case level >= 4 && level <= MaxLevel:
		// 4次以降は3次を2分割していく。4次=15秒、5次=7.5秒、以降は半分ずつ。
		half := math.Exp2(float64(level - 3))
		return 1.0 / 120.0 / half, 1.0 / 80.0 / half, nil
	}
	return 0, 0, fmt.Errorf("メッシュ次数は1〜%dの範囲: %d", MaxLevel, level)
}

// Digits は指定次数のメッシュコードの桁数を返す。未対応の次数では0。
func Digits(level int) int {
	switch {
	case level == 1:
		return 4
	case level == 2:
		return 6
	case level == 3:
		return 8
	case level >= 4 && level <= MaxLevel:
		// 4次以降は分割番号を1桁ずつ足していく。
		return 8 + (level - 3)
	}
	return 0
}

// LevelForCode は桁数からメッシュ次数を判定する。
func LevelForCode(code string) (int, error) {
	for level := 1; level <= MaxLevel; level++ {
		if Digits(level) == len(code) {
			return level, nil
		}
	}
	return 0, fmt.Errorf("メッシュコードの桁数が不正: %q", code)
}

// Encode は緯度経度を含むメッシュの、指定次数でのコードを返す。
func Encode(p LatLon, level int) (string, error) {
	if _, _, err := Span(level); err != nil {
		return "", err
	}
	if p.Lat <= 0 || p.Lat >= 66 || p.Lon <= 100 || p.Lon >= 180 {
		return "", fmt.Errorf("標準地域メッシュの適用範囲外: %+v", p)
	}

	// 緯度は1.5倍すると1次メッシュの整数インデックスになる。
	// 経度は100を引いた整数部がそのまま1次メッシュのインデックス。
	fLat := p.Lat * 1.5
	fLon := p.Lon - 100

	lat1 := math.Floor(fLat)
	lon1 := math.Floor(fLon)
	code := fmt.Sprintf("%02d%02d", int(lat1), int(lon1))
	if level == 1 {
		return code, nil
	}

	// 2次メッシュ: 1次を8×8に分割。
	fLat, fLon = (fLat-lat1)*8, (fLon-lon1)*8
	lat2, lon2 := math.Floor(fLat), math.Floor(fLon)
	code += fmt.Sprintf("%d%d", int(lat2), int(lon2))
	if level == 2 {
		return code, nil
	}

	// 3次メッシュ: 2次を10×10に分割。
	fLat, fLon = (fLat-lat2)*10, (fLon-lon2)*10
	lat3, lon3 := math.Floor(fLat), math.Floor(fLon)
	code += fmt.Sprintf("%d%d", int(lat3), int(lon3))
	if level == 3 {
		return code, nil
	}

	// 4次以降: 直前の区画を2×2に分割し、
	// 南西=1 南東=2 北西=3 北東=4 の1桁を次数の数だけ足していく。
	prevLat, prevLon := lat3, lon3
	for l := 4; l <= level; l++ {
		fLat, fLon = (fLat-prevLat)*2, (fLon-prevLon)*2
		prevLat, prevLon = math.Floor(fLat), math.Floor(fLon)
		code += strconv.Itoa(int(prevLat)*2 + int(prevLon) + 1)
	}
	return code, nil
}

// Cell はメッシュ1区画を表す。
type Cell struct {
	Code    string
	Level   int
	SW      LatLon  // 南西端
	LatSpan float64 // 緯度方向の幅（度）
	LonSpan float64 // 経度方向の幅（度）
}

// Center は区画の中心座標を返す。
func (c Cell) Center() LatLon {
	return LatLon{Lat: c.SW.Lat + c.LatSpan/2, Lon: c.SW.Lon + c.LonSpan/2}
}

// Decode はメッシュコードを区画に変換する。
func Decode(code string) (Cell, error) {
	level, err := LevelForCode(code)
	if err != nil {
		return Cell{}, err
	}
	digits := make([]int, len(code))
	for i, r := range code {
		if r < '0' || r > '9' {
			return Cell{}, fmt.Errorf("メッシュコードに数字以外が含まれる: %q", code)
		}
		digits[i] = int(r - '0')
	}

	lat := float64(digits[0]*10+digits[1]) * (2.0 / 3.0)
	lon := float64(digits[2]*10+digits[3]) + 100
	latSpan, lonSpan := 2.0/3.0, 1.0

	if level >= 2 {
		latSpan, lonSpan = latSpan/8, lonSpan/8
		lat += float64(digits[4]) * latSpan
		lon += float64(digits[5]) * lonSpan
	}
	if level >= 3 {
		latSpan, lonSpan = latSpan/10, lonSpan/10
		lat += float64(digits[6]) * latSpan
		lon += float64(digits[7]) * lonSpan
	}
	for i, l := 8, 4; l <= level; i, l = i+1, l+1 {
		quadrant := digits[i]
		if quadrant < 1 || quadrant > 4 {
			return Cell{}, fmt.Errorf("%d次メッシュの分割番号が不正: %q", l, code)
		}
		latSpan, lonSpan = latSpan/2, lonSpan/2
		lat += float64((quadrant-1)/2) * latSpan
		lon += float64((quadrant-1)%2) * lonSpan
	}

	return Cell{Code: code, Level: level, SW: LatLon{Lat: lat, Lon: lon}, LatSpan: latSpan, LonSpan: lonSpan}, nil
}

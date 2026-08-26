// Package geomesh は日本の標準地域メッシュ（JIS X 0410）のメッシュコードと
// 緯度経度を相互変換する。
//
// 5次メッシュ（約250m四方）がそのままワールドマップのタイル1マスに対応するため、
// メッシュコードの格子構造をタイルグリッドの座標系として利用する。
package geomesh

import (
	"fmt"
	"math"
	"strconv"
)

// MaxLevel は対応する最大メッシュ次数。5次 = 1/4地域メッシュ = 約250m。
const MaxLevel = 5

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
	switch level {
	case 1:
		return 2.0 / 3.0, 1.0, nil // 40分 × 1度
	case 2:
		return 1.0 / 12.0, 1.0 / 8.0, nil // 5分 × 7分30秒
	case 3:
		return 1.0 / 120.0, 1.0 / 80.0, nil // 30秒 × 45秒
	case 4:
		return 1.0 / 240.0, 1.0 / 160.0, nil // 15秒 × 22.5秒
	case 5:
		return 1.0 / 480.0, 1.0 / 320.0, nil // 7.5秒 × 11.25秒
	}
	return 0, 0, fmt.Errorf("メッシュ次数は1〜%dの範囲: %d", MaxLevel, level)
}

// Digits は指定次数のメッシュコードの桁数を返す。未対応の次数では0。
func Digits(level int) int {
	switch level {
	case 1:
		return 4
	case 2:
		return 6
	case 3:
		return 8
	case 4:
		return 9
	case 5:
		return 10
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

	// 4次・5次メッシュ: 直前の区画を2×2に分割し、
	// 南西=1 南東=2 北西=3 北東=4 の1桁で表す。
	fLat, fLon = (fLat-lat3)*2, (fLon-lon3)*2
	lat4, lon4 := math.Floor(fLat), math.Floor(fLon)
	code += strconv.Itoa(int(lat4)*2 + int(lon4) + 1)
	if level == 4 {
		return code, nil
	}

	fLat, fLon = (fLat-lat4)*2, (fLon-lon4)*2
	lat5, lon5 := math.Floor(fLat), math.Floor(fLon)
	code += strconv.Itoa(int(lat5)*2 + int(lon5) + 1)
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

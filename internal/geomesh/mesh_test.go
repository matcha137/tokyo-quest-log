package geomesh

import (
	"math"
	"testing"
)

// 実在地点の3次メッシュコードは公開データと照合できるため、
// 変換の正しさを固定できる。
func TestEncodeKnownLandmarks(t *testing.T) {
	cases := []struct {
		name  string
		point LatLon
		level int
		want  string
	}{
		{"東京駅 1次", LatLon{35.6812, 139.7671}, 1, "5339"},
		{"東京駅 2次", LatLon{35.6812, 139.7671}, 2, "533946"},
		{"東京駅 3次", LatLon{35.6812, 139.7671}, 3, "53394611"},
		{"新宿駅 3次", LatLon{35.6896, 139.7006}, 3, "53394526"},
		{"雲取山 2次", LatLon{35.8553, 138.9436}, 2, "533867"},
	}
	for _, tc := range cases {
		got, err := Encode(tc.point, tc.level)
		if err != nil {
			t.Fatalf("%s: %v", tc.name, err)
		}
		if got != tc.want {
			t.Errorf("%s: Encode = %q, want %q", tc.name, got, tc.want)
		}
	}
}

// Encode と Decode が往復すること。Decode した区画に元の点が含まれ、
// その中心を再び Encode すると同じコードに戻る。
func TestEncodeDecodeRoundTrip(t *testing.T) {
	points := []LatLon{
		{35.6812, 139.7671}, // 東京駅
		{35.5494, 139.7798}, // 羽田空港
		{35.6259, 139.2434}, // 高尾山
		{35.7900, 139.0000}, // 奥多摩
	}
	for _, p := range points {
		for level := 1; level <= MaxLevel; level++ {
			code, err := Encode(p, level)
			if err != nil {
				t.Fatalf("Encode(%v, %d): %v", p, level, err)
			}
			cell, err := Decode(code)
			if err != nil {
				t.Fatalf("Decode(%q): %v", code, err)
			}
			if cell.Level != level {
				t.Errorf("Decode(%q).Level = %d, want %d", code, cell.Level, level)
			}
			if p.Lat < cell.SW.Lat || p.Lat >= cell.SW.Lat+cell.LatSpan {
				t.Errorf("%q: 緯度 %v が区画 [%v, %v) の外", code, p.Lat, cell.SW.Lat, cell.SW.Lat+cell.LatSpan)
			}
			if p.Lon < cell.SW.Lon || p.Lon >= cell.SW.Lon+cell.LonSpan {
				t.Errorf("%q: 経度 %v が区画 [%v, %v) の外", code, p.Lon, cell.SW.Lon, cell.SW.Lon+cell.LonSpan)
			}
			again, err := Encode(cell.Center(), level)
			if err != nil {
				t.Fatalf("Encode(center of %q): %v", code, err)
			}
			if again != code {
				t.Errorf("中心の再変換 = %q, want %q", again, code)
			}
		}
	}
}

// 5次メッシュの実寸。ワールドマップの縮尺設計の前提になるため固定しておく。
func TestLevel5SpanMeters(t *testing.T) {
	latSpan, lonSpan, err := Span(5)
	if err != nil {
		t.Fatal(err)
	}
	const degreeMeters = 111_320.0
	latMeters := latSpan * degreeMeters
	lonMeters := lonSpan * degreeMeters * math.Cos(35.7*math.Pi/180)
	if latMeters < 225 || latMeters > 240 {
		t.Errorf("5次メッシュの南北 = %.1fm, 約232mを期待", latMeters)
	}
	if lonMeters < 275 || lonMeters > 290 {
		t.Errorf("5次メッシュの東西 = %.1fm, 約282mを期待", lonMeters)
	}
	t.Logf("5次メッシュ実寸: 南北 %.1fm × 東西 %.1fm", latMeters, lonMeters)
}

func TestDecodeRejectsInvalid(t *testing.T) {
	for _, code := range []string{"", "533", "5339461", "5339461a", "533946115"} {
		if _, err := Decode(code); err == nil {
			t.Errorf("Decode(%q) がエラーにならなかった", code)
		}
	}
}

// 6次以降の拡張でも往復変換が保たれること。
func TestFineLevelsRoundTrip(t *testing.T) {
	p := LatLon{Lat: 35.6812, Lon: 139.7671} // 東京駅
	for level := StandardMaxLevel + 1; level <= MaxLevel; level++ {
		code, err := Encode(p, level)
		if err != nil {
			t.Fatalf("Encode(level %d): %v", level, err)
		}
		if len(code) != Digits(level) {
			t.Errorf("%d次の桁数 = %d, want %d", level, len(code), Digits(level))
		}
		cell, err := Decode(code)
		if err != nil {
			t.Fatalf("Decode(%q): %v", code, err)
		}
		if cell.Level != level {
			t.Errorf("Decode(%q).Level = %d, want %d", code, cell.Level, level)
		}
		if p.Lat < cell.SW.Lat || p.Lat >= cell.SW.Lat+cell.LatSpan {
			t.Errorf("%q: 緯度が区画の外", code)
		}
		if p.Lon < cell.SW.Lon || p.Lon >= cell.SW.Lon+cell.LonSpan {
			t.Errorf("%q: 経度が区画の外", code)
		}
		if again, _ := Encode(cell.Center(), level); again != code {
			t.Errorf("中心の再変換 = %q, want %q", again, code)
		}
	}
}

// 次数が1つ上がるごとに区画がちょうど半分になること。
func TestFineLevelsHalveEachStep(t *testing.T) {
	for level := 4; level < MaxLevel; level++ {
		lat, lon, err := Span(level)
		if err != nil {
			t.Fatal(err)
		}
		nextLat, nextLon, err := Span(level + 1)
		if err != nil {
			t.Fatal(err)
		}
		if math.Abs(nextLat*2-lat) > 1e-12 || math.Abs(nextLon*2-lon) > 1e-12 {
			t.Errorf("%d次から%d次で半分になっていない", level, level+1)
		}
	}
}

// 街のマップに使う細かい次数の実寸。縮尺設計の前提として固定しておく。
func TestFineLevelSpanMeters(t *testing.T) {
	const degreeMeters = 111_320.0
	for _, tc := range []struct {
		level            int
		wantLat, wantLon float64
	}{
		{8, 29, 35},
		{9, 14, 18},
		{10, 7, 9},
	} {
		latSpan, lonSpan, err := Span(tc.level)
		if err != nil {
			t.Fatal(err)
		}
		latMeters := latSpan * degreeMeters
		lonMeters := lonSpan * degreeMeters * math.Cos(35.68*math.Pi/180)
		if math.Abs(latMeters-tc.wantLat) > 2 || math.Abs(lonMeters-tc.wantLon) > 2 {
			t.Errorf("%d次 = %.1fm × %.1fm, 約%.0fm × %.0fm を期待",
				tc.level, latMeters, lonMeters, tc.wantLat, tc.wantLon)
		}
		t.Logf("%2d次メッシュ: 南北 %5.1fm × 東西 %5.1fm", tc.level, latMeters, lonMeters)
	}
}

func TestSpanRejectsTooFine(t *testing.T) {
	if _, _, err := Span(MaxLevel + 1); err == nil {
		t.Error("上限を超える次数がエラーにならなかった")
	}
}

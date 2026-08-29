// p12tourism は国土数値情報の観光資源データ（P12）のGMLを読み、
// ランドマークJSONへ変換する。
//
//	go run ./tools/p12tourism -in data/p12/P12-14_13.xml -out assets/tourism.json
//
// 出力は assets/landmarks.json と同じ形式で、meshbuild の -landmarks に渡せる。
//
// 出典: 国土数値情報 観光資源データ（国土交通省）
package main

import (
	"encoding/json"
	"encoding/xml"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"

	"tokyo-quest-log/internal/landmark"
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, "p12tourism:", err)
		os.Exit(1)
	}
}

func run() error {
	in := flag.String("in", "", "P12のGML(XML)のパス（必須）")
	out := flag.String("out", "assets/tourism.json", "出力するランドマークJSON")
	flag.Parse()
	if *in == "" {
		flag.Usage()
		return fmt.Errorf("-in は必須です")
	}

	raw, err := os.ReadFile(*in)
	if err != nil {
		return err
	}
	var doc dataset
	if err := xml.Unmarshal(raw, &doc); err != nil {
		return fmt.Errorf("%s: %w", *in, err)
	}

	spots, dropped := doc.spots()
	if len(spots) == 0 {
		return fmt.Errorf("%s: 観光資源が1件も読めませんでした", *in)
	}
	if err := writeJSON(*out, spots); err != nil {
		return err
	}

	fmt.Printf("観光資源を変換しました\n")
	fmt.Printf("  点   : %d 件\n", len(doc.ResourcePoints))
	fmt.Printf("  面   : %d 件（外周の中心を代表点にしました）\n", len(doc.ResourceSurfaces))
	fmt.Printf("  出力 : %d 件 -> %s\n", len(spots), *out)
	if len(dropped) > 0 {
		fmt.Printf("  除外 : %d 件（同名の重複、または座標が引けないもの）\n", len(dropped))
		fmt.Printf("         %s\n", strings.Join(dropped, ", "))
	}
	fmt.Printf("\n出典: 国土数値情報 観光資源データ（国土交通省）\n")
	return nil
}

// GMLの構造。名前空間は無視し、要素名だけで対応づける。
type dataset struct {
	Points           []gmlPoint        `xml:"Point"`
	Curves           []gmlCurve        `xml:"Curve"`
	Surfaces         []gmlSurface      `xml:"Surface"`
	ResourcePoints   []resourceFeature `xml:"TourismResource_Point"`
	ResourceSurfaces []resourceFeature `xml:"TourismResource_Surface"`
}

type gmlPoint struct {
	ID  string `xml:"id,attr"`
	Pos string `xml:"pos"`
}

type gmlCurve struct {
	ID      string `xml:"id,attr"`
	PosList string `xml:"segments>LineStringSegment>posList"`
}

// xlinkRef は xlink:href だけを持つ参照要素。
// 属性は入れ子のパス指定では読めないため、要素ごとに型を用意する。
type xlinkRef struct {
	Href string `xml:"href,attr"`
}

type gmlSurface struct {
	ID          string   `xml:"id,attr"`
	CurveMember xlinkRef `xml:"patches>PolygonPatch>exterior>Ring>curveMember"`
}

type resourceFeature struct {
	ID       string   `xml:"id,attr"`
	Name     string   `xml:"turismResorceName"` // スキーマ側の綴りに合わせる
	Kind     string   `xml:"turismResorceKindName"`
	Address  string   `xml:"address"`
	Position xlinkRef `xml:"position"`
	Bounds   xlinkRef `xml:"bounds"`
}

// spots は観光資源をランドマークへ変換する。
// 点と面に同じ資源が重複して収録されているため、名前で1件にまとめる。
func (d *dataset) spots() (spots []landmark.Landmark, dropped []string) {
	points := map[string]gmlPoint{}
	for _, p := range d.Points {
		points[p.ID] = p
	}
	curves := map[string]gmlCurve{}
	for _, c := range d.Curves {
		curves[c.ID] = c
	}
	surfaces := map[string]gmlSurface{}
	for _, s := range d.Surfaces {
		surfaces[s.ID] = s
	}

	seen := map[string]bool{}
	// 点を先に処理する。面より代表点として素直なため。
	for _, group := range [][]resourceFeature{d.ResourcePoints, d.ResourceSurfaces} {
		for _, f := range group {
			if f.Name == "" || seen[f.Name] {
				if f.Name != "" {
					dropped = append(dropped, f.Name)
				}
				continue
			}
			lat, lon, ok := locate(f, points, curves, surfaces)
			if !ok {
				dropped = append(dropped, f.Name)
				continue
			}
			seen[f.Name] = true
			spots = append(spots, landmark.Landmark{
				ID:   "p12-" + f.ID,
				Name: f.Name,
				Kind: kindOf(f.Kind),
				Lat:  lat,
				Lon:  lon,
				Note: strings.TrimSpace(f.Kind + "  " + f.Address),
			})
		}
	}
	sort.Slice(spots, func(i, j int) bool { return spots[i].ID < spots[j].ID })
	return spots, dropped
}

func locate(f resourceFeature, points map[string]gmlPoint, curves map[string]gmlCurve, surfaces map[string]gmlSurface) (lat, lon float64, ok bool) {
	if ref := trimHash(f.Position.Href); ref != "" {
		if p, found := points[ref]; found {
			return parsePos(p.Pos)
		}
	}
	if ref := trimHash(f.Bounds.Href); ref != "" {
		s, found := surfaces[ref]
		if !found {
			return 0, 0, false
		}
		c, found := curves[trimHash(s.CurveMember.Href)]
		if !found {
			return 0, 0, false
		}
		return centerOf(c.PosList)
	}
	return 0, 0, false
}

func trimHash(href string) string { return strings.TrimPrefix(strings.TrimSpace(href), "#") }

// parsePos は「緯度 経度」形式の1点を読む。
func parsePos(pos string) (lat, lon float64, ok bool) {
	fields := strings.Fields(pos)
	if len(fields) < 2 {
		return 0, 0, false
	}
	lat, err1 := strconv.ParseFloat(fields[0], 64)
	lon, err2 := strconv.ParseFloat(fields[1], 64)
	if err1 != nil || err2 != nil {
		return 0, 0, false
	}
	return lat, lon, true
}

// centerOf は外周の外接矩形の中心を代表点として返す。
// 面のままではマス1つに落とせないため、地図上の目印として1点に畳む。
func centerOf(posList string) (lat, lon float64, ok bool) {
	fields := strings.Fields(posList)
	if len(fields) < 4 {
		return 0, 0, false
	}
	minLat, maxLat := 90.0, -90.0
	minLon, maxLon := 180.0, -180.0
	for i := 0; i+1 < len(fields); i += 2 {
		a, err1 := strconv.ParseFloat(fields[i], 64)
		b, err2 := strconv.ParseFloat(fields[i+1], 64)
		if err1 != nil || err2 != nil {
			continue
		}
		minLat, maxLat = min(minLat, a), max(maxLat, a)
		minLon, maxLon = min(minLon, b), max(maxLon, b)
	}
	if minLat > maxLat {
		return 0, 0, false
	}
	return (minLat + maxLat) / 2, (minLon + maxLon) / 2, true
}

// kindOf はP12の資源種別をゲーム側の種別へ寄せる。
// 元の種別名は note に残すため、ここでは大きく2つに分けるだけにする。
func kindOf(p12Kind string) landmark.Kind {
	if strings.Contains(p12Kind, "神社") || strings.Contains(p12Kind, "寺院") || strings.Contains(p12Kind, "教会") {
		return landmark.KindShrine
	}
	return landmark.KindSight
}

func writeJSON(path string, spots []landmark.Landmark) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	doc := struct {
		Note      string              `json:"note"`
		Landmarks []landmark.Landmark `json:"landmarks"`
	}{
		Note:      "国土数値情報 観光資源データ（P12, 2014年版）から自動生成。手で編集せず、p12tourism を再実行すること。",
		Landmarks: spots,
	}
	raw, err := json.MarshalIndent(doc, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, append(raw, '\n'), 0o644)
}

// Package geojson は水域・行政区域の重ね合わせに必要な範囲で GeoJSON を読む。
//
// 面（Polygon / MultiPolygon）と線（LineString / MultiLineString）だけを扱い、
// 属性は読み飛ばす。ワールドマップの塗り分けに必要なのは形状のみで、
// 属性による絞り込みは事前に ogr2ogr などで済ませる前提。
package geojson

import (
	"encoding/json"
	"fmt"
	"io"
	"math"
)

// Point は経度緯度。GeoJSON の座標順（[lon, lat]）に合わせる。
type Point struct {
	Lon float64
	Lat float64
}

// Ring は閉じた輪。Polygon の最初の Ring が外周、以降が穴。
type Ring []Point

// Polygon は外周と穴の集まり。
type Polygon []Ring

// Line は折れ線。
type Line []Point

// Collection は読み込んだ形状をまとめたもの。
type Collection struct {
	Polygons []Polygon
	Lines    []Line
}

// Empty は形状が1つも無いかを返す。
func (c Collection) Empty() bool {
	return len(c.Polygons) == 0 && len(c.Lines) == 0
}

// Bounds は全形状を覆う矩形を返す。形状が無い場合は ok=false。
func (c Collection) Bounds() (minLon, minLat, maxLon, maxLat float64, ok bool) {
	minLon, minLat = math.Inf(1), math.Inf(1)
	maxLon, maxLat = math.Inf(-1), math.Inf(-1)
	visit := func(p Point) {
		ok = true
		minLon, maxLon = math.Min(minLon, p.Lon), math.Max(maxLon, p.Lon)
		minLat, maxLat = math.Min(minLat, p.Lat), math.Max(maxLat, p.Lat)
	}
	for _, poly := range c.Polygons {
		for _, ring := range poly {
			for _, p := range ring {
				visit(p)
			}
		}
	}
	for _, line := range c.Lines {
		for _, p := range line {
			visit(p)
		}
	}
	return
}

// node は GeoJSON の各種オブジェクトを緩く受ける。
type node struct {
	Type        string          `json:"type"`
	Coordinates json.RawMessage `json:"coordinates"`
	Geometries  []node          `json:"geometries"`
	Geometry    *node           `json:"geometry"`
	Features    []node          `json:"features"`
}

// Parse は FeatureCollection / Feature / Geometry のいずれでも受け付ける。
func Parse(r io.Reader) (Collection, error) {
	var root node
	if err := json.NewDecoder(r).Decode(&root); err != nil {
		return Collection{}, fmt.Errorf("GeoJSONの解析: %w", err)
	}
	var c Collection
	if err := c.walk(&root); err != nil {
		return Collection{}, err
	}
	return c, nil
}

func (c *Collection) walk(n *node) error {
	if n == nil {
		return nil
	}
	switch n.Type {
	case "FeatureCollection":
		for i := range n.Features {
			if err := c.walk(&n.Features[i]); err != nil {
				return err
			}
		}
		return nil
	case "Feature":
		return c.walk(n.Geometry)
	case "GeometryCollection":
		for i := range n.Geometries {
			if err := c.walk(&n.Geometries[i]); err != nil {
				return err
			}
		}
		return nil
	case "Point", "MultiPoint":
		return nil // 面・線のみを扱う
	case "LineString":
		var line Line
		if err := decodeCoords(n.Coordinates, &line); err != nil {
			return err
		}
		if len(line) >= 2 {
			c.Lines = append(c.Lines, line)
		}
		return nil
	case "MultiLineString":
		var lines []Line
		if err := decodeCoords(n.Coordinates, &lines); err != nil {
			return err
		}
		for _, line := range lines {
			if len(line) >= 2 {
				c.Lines = append(c.Lines, line)
			}
		}
		return nil
	case "Polygon":
		var poly Polygon
		if err := decodeCoords(n.Coordinates, &poly); err != nil {
			return err
		}
		if len(poly) > 0 {
			c.Polygons = append(c.Polygons, poly)
		}
		return nil
	case "MultiPolygon":
		var polys []Polygon
		if err := decodeCoords(n.Coordinates, &polys); err != nil {
			return err
		}
		for _, poly := range polys {
			if len(poly) > 0 {
				c.Polygons = append(c.Polygons, poly)
			}
		}
		return nil
	case "":
		return fmt.Errorf("GeoJSONに type がない")
	}
	return fmt.Errorf("未対応のGeoJSON型: %q", n.Type)
}

func decodeCoords(raw json.RawMessage, dst any) error {
	if len(raw) == 0 {
		return fmt.Errorf("coordinates が空")
	}
	if err := json.Unmarshal(raw, dst); err != nil {
		return fmt.Errorf("coordinates の解析: %w", err)
	}
	return nil
}

// UnmarshalJSON は GeoJSON の [lon, lat] 形式を読む。
// 3要素目（標高）が付いていても無視する。
func (p *Point) UnmarshalJSON(data []byte) error {
	var values []float64
	if err := json.Unmarshal(data, &values); err != nil {
		return err
	}
	if len(values) < 2 {
		return fmt.Errorf("座標の要素数が足りない: %v", values)
	}
	p.Lon, p.Lat = values[0], values[1]
	return nil
}

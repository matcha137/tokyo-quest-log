package geojson

import (
	"strings"
	"testing"
)

const featureCollection = `{
  "type": "FeatureCollection",
  "features": [
    {"type": "Feature", "properties": {"name": "多摩川"},
     "geometry": {"type": "LineString", "coordinates": [[139.1,35.6],[139.4,35.55],[139.75,35.53]]}},
    {"type": "Feature", "properties": {"name": "奥多摩湖"},
     "geometry": {"type": "Polygon", "coordinates": [
        [[139.0,35.78],[139.1,35.78],[139.1,35.80],[139.0,35.80],[139.0,35.78]],
        [[139.03,35.785],[139.06,35.785],[139.06,35.795],[139.03,35.795],[139.03,35.785]]]}},
    {"type": "Feature", "properties": {},
     "geometry": {"type": "Point", "coordinates": [139.767,35.681]}}
  ]
}`

func TestParseFeatureCollection(t *testing.T) {
	c, err := Parse(strings.NewReader(featureCollection))
	if err != nil {
		t.Fatal(err)
	}
	if len(c.Lines) != 1 {
		t.Fatalf("線の数 = %d, want 1", len(c.Lines))
	}
	if len(c.Lines[0]) != 3 {
		t.Errorf("線の頂点数 = %d, want 3", len(c.Lines[0]))
	}
	if got := c.Lines[0][0]; got.Lon != 139.1 || got.Lat != 35.6 {
		t.Errorf("座標順が [lon, lat] になっていない: %+v", got)
	}
	if len(c.Polygons) != 1 {
		t.Fatalf("面の数 = %d, want 1", len(c.Polygons))
	}
	if len(c.Polygons[0]) != 2 {
		t.Errorf("輪の数 = %d, want 2（外周と穴）", len(c.Polygons[0]))
	}
}

func TestParseMultiGeometries(t *testing.T) {
	const doc = `{"type":"GeometryCollection","geometries":[
	  {"type":"MultiPolygon","coordinates":[
	     [[[0,0],[1,0],[1,1],[0,0]]],
	     [[[2,2],[3,2],[3,3],[2,2]]]]},
	  {"type":"MultiLineString","coordinates":[[[0,0],[1,1]],[[2,2],[3,3]]]}]}`
	c, err := Parse(strings.NewReader(doc))
	if err != nil {
		t.Fatal(err)
	}
	if len(c.Polygons) != 2 {
		t.Errorf("面の数 = %d, want 2", len(c.Polygons))
	}
	if len(c.Lines) != 2 {
		t.Errorf("線の数 = %d, want 2", len(c.Lines))
	}
}

// 3要素目の標高が付いていても読めること。
func TestParseAcceptsElevationInCoordinates(t *testing.T) {
	const doc = `{"type":"LineString","coordinates":[[139.1,35.6,12.5],[139.2,35.7,20.0]]}`
	c, err := Parse(strings.NewReader(doc))
	if err != nil {
		t.Fatal(err)
	}
	if len(c.Lines) != 1 || c.Lines[0][1].Lat != 35.7 {
		t.Errorf("標高付き座標を読めていない: %+v", c.Lines)
	}
}

func TestBounds(t *testing.T) {
	c, err := Parse(strings.NewReader(featureCollection))
	if err != nil {
		t.Fatal(err)
	}
	minLon, minLat, maxLon, maxLat, ok := c.Bounds()
	if !ok {
		t.Fatal("範囲が得られない")
	}
	if minLon != 139.0 || maxLon != 139.75 {
		t.Errorf("経度範囲 = %v〜%v", minLon, maxLon)
	}
	if minLat != 35.53 || maxLat != 35.80 {
		t.Errorf("緯度範囲 = %v〜%v", minLat, maxLat)
	}
	if (Collection{}).Empty() != true {
		t.Error("空の Collection が Empty でない")
	}
}

func TestParseErrors(t *testing.T) {
	for name, doc := range map[string]string{
		"壊れたJSON":       `{"type":`,
		"typeなし":        `{"coordinates":[[0,0],[1,1]]}`,
		"未対応の型":         `{"type":"Sphere","coordinates":[0,0]}`,
		"座標の要素不足":       `{"type":"LineString","coordinates":[[0],[1]]}`,
		"coordinates欠落": `{"type":"Polygon"}`,
	} {
		if _, err := Parse(strings.NewReader(doc)); err == nil {
			t.Errorf("%s: エラーにならなかった", name)
		}
	}
}

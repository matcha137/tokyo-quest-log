package tmx

import (
	"bytes"
	"encoding/xml"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"tokyo-quest-log/internal/landmark"
	"tokyo-quest-log/internal/worldgrid"
)

func testGrid(t *testing.T) *worldgrid.Grid {
	t.Helper()
	g, err := worldgrid.New(worldgrid.Bounds{MinLat: 35.60, MinLon: 139.60, MaxLat: 35.62, MaxLon: 139.63}, 5)
	if err != nil {
		t.Fatal(err)
	}
	for i := range g.Tiles {
		g.Tiles[i] = worldgrid.Terrain(i % int(worldgrid.TerrainCount))
	}
	return g
}

// 書き出したTMXが正しいXMLで、寸法とタイル数が一致すること。
func TestExportStructure(t *testing.T) {
	g := testGrid(t)
	marks := []landmark.Placed{
		{Landmark: landmark.Landmark{ID: "a", Name: "テスト町", Kind: landmark.KindTown, Lat: 35.61, Lon: 139.61}, Row: 2, Col: 3},
	}
	var buf bytes.Buffer
	if err := Export(&buf, g, marks, Options{TileSize: 32}); err != nil {
		t.Fatal(err)
	}

	var got struct {
		Width      int `xml:"width,attr"`
		Height     int `xml:"height,attr"`
		TileWidth  int `xml:"tilewidth,attr"`
		Properties []struct {
			Name  string `xml:"name,attr"`
			Value string `xml:"value,attr"`
		} `xml:"properties>property"`
		Tileset struct {
			TileCount int `xml:"tilecount,attr"`
			Image     struct {
				Source string `xml:"source,attr"`
				Width  int    `xml:"width,attr"`
			} `xml:"image"`
		} `xml:"tileset"`
		Layer struct {
			Data struct {
				Encoding string `xml:"encoding,attr"`
				CSV      string `xml:",chardata"`
			} `xml:"data"`
		} `xml:"layer"`
		Objects []struct {
			Name string  `xml:"name,attr"`
			Type string  `xml:"type,attr"`
			X    float64 `xml:"x,attr"`
			Y    float64 `xml:"y,attr"`
		} `xml:"objectgroup>object"`
	}
	if err := xml.Unmarshal(buf.Bytes(), &got); err != nil {
		t.Fatalf("書き出したTMXを読み戻せない: %v", err)
	}

	if got.Width != g.Cols || got.Height != g.Rows {
		t.Errorf("寸法 = %d×%d, want %d×%d", got.Width, got.Height, g.Cols, g.Rows)
	}
	if got.Tileset.TileCount != int(worldgrid.TerrainCount) {
		t.Errorf("タイル種別数 = %d, want %d", got.Tileset.TileCount, worldgrid.TerrainCount)
	}
	if want := int(worldgrid.TerrainCount) * 32; got.Tileset.Image.Width != want {
		t.Errorf("タイルセット画像幅 = %d, want %d", got.Tileset.Image.Width, want)
	}
	if got.Layer.Data.Encoding != "csv" {
		t.Errorf("エンコーディング = %q, want csv", got.Layer.Data.Encoding)
	}

	// マップのプロパティから緯度経度へ戻せること。
	props := map[string]string{}
	for _, p := range got.Properties {
		props[p.Name] = p.Value
	}
	if props["originLat"] == "" || props["latSpan"] == "" {
		t.Errorf("原点の情報がプロパティに残っていない: %v", props)
	}

	if len(got.Objects) != 1 {
		t.Fatalf("オブジェクト数 = %d, want 1", len(got.Objects))
	}
	if got.Objects[0].Name != "テスト町" || got.Objects[0].Type != "town" {
		t.Errorf("オブジェクトが不正: %+v", got.Objects[0])
	}
	if got.Objects[0].X != 3*32 || got.Objects[0].Y != 2*32 {
		t.Errorf("オブジェクト座標 = (%v,%v), want (96,64)", got.Objects[0].X, got.Objects[0].Y)
	}
}

// CSVのGIDが「地形の値+1」で、マス数と並び順が一致すること。
func TestExportCSVMatchesGrid(t *testing.T) {
	g := testGrid(t)
	var buf bytes.Buffer
	if err := Export(&buf, g, nil, Options{}); err != nil {
		t.Fatal(err)
	}
	var got struct {
		CSV string `xml:"layer>data"`
	}
	if err := xml.Unmarshal(buf.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	fields := strings.Split(strings.TrimSpace(strings.ReplaceAll(got.CSV, "\n", "")), ",")
	if len(fields) != g.Cols*g.Rows {
		t.Fatalf("CSVの要素数 = %d, want %d", len(fields), g.Cols*g.Rows)
	}
	for i, field := range fields {
		gid, err := strconv.Atoi(strings.TrimSpace(field))
		if err != nil {
			t.Fatalf("%d番目が数値でない: %q", i, field)
		}
		if want := int(g.Tiles[i]) + 1; gid != want {
			t.Fatalf("%d番目のGID = %d, want %d", i, gid, want)
		}
	}
}

func TestExportWithoutLandmarks(t *testing.T) {
	var buf bytes.Buffer
	if err := Export(&buf, testGrid(t), nil, Options{}); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(buf.String(), "<objectgroup") {
		t.Error("ランドマークが無いのにオブジェクトレイヤーが出力された")
	}
}

func TestWriteTilesetPNG(t *testing.T) {
	path := filepath.Join(t.TempDir(), "terrain.png")
	if err := WriteTilesetPNG(path, 16); err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(path)
	if err != nil || info.Size() == 0 {
		t.Fatalf("タイルセット画像が生成されていない: %v", err)
	}
}

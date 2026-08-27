// Package tmx は生成したワールドマップを Tiled の TMX 形式で書き出す。
//
// 標高から機械的に作った地形はあくまで下敷きで、実際のゲームマップは
// Tiled で手直しして仕上げる想定。そのための受け渡し形式。
package tmx

import (
	"encoding/xml"
	"fmt"
	"image"
	"image/color"
	"image/png"
	"io"
	"os"
	"strconv"
	"strings"

	"tokyo-quest-log/internal/landmark"
	"tokyo-quest-log/internal/worldgrid"
)

// Options は書き出しの設定。
type Options struct {
	TileSize     int    // 1マスのピクセル数
	TilesetImage string // TMXから参照するタイルセット画像のファイル名
	TilesetName  string
}

func (o Options) withDefaults() Options {
	if o.TileSize <= 0 {
		o.TileSize = 32
	}
	if o.TilesetImage == "" {
		o.TilesetImage = "terrain.png"
	}
	if o.TilesetName == "" {
		o.TilesetName = "terrain"
	}
	return o
}

type tmxImage struct {
	Source string `xml:"source,attr"`
	Width  int    `xml:"width,attr"`
	Height int    `xml:"height,attr"`
}

type property struct {
	Name  string `xml:"name,attr"`
	Type  string `xml:"type,attr,omitempty"`
	Value string `xml:"value,attr"`
}

type tile struct {
	ID         int        `xml:"id,attr"`
	Type       string     `xml:"type,attr,omitempty"`
	Properties []property `xml:"properties>property,omitempty"`
}

type tileset struct {
	FirstGID   int      `xml:"firstgid,attr"`
	Name       string   `xml:"name,attr"`
	TileWidth  int      `xml:"tilewidth,attr"`
	TileHeight int      `xml:"tileheight,attr"`
	TileCount  int      `xml:"tilecount,attr"`
	Columns    int      `xml:"columns,attr"`
	Image      tmxImage `xml:"image"`
	Tiles      []tile   `xml:"tile"`
}

type layerData struct {
	Encoding string `xml:"encoding,attr"`
	CSV      string `xml:",chardata"`
}

type layer struct {
	ID     int       `xml:"id,attr"`
	Name   string    `xml:"name,attr"`
	Width  int       `xml:"width,attr"`
	Height int       `xml:"height,attr"`
	Data   layerData `xml:"data"`
}

type object struct {
	ID         int        `xml:"id,attr"`
	Name       string     `xml:"name,attr"`
	Type       string     `xml:"type,attr,omitempty"`
	X          float64    `xml:"x,attr"`
	Y          float64    `xml:"y,attr"`
	Width      float64    `xml:"width,attr"`
	Height     float64    `xml:"height,attr"`
	Properties []property `xml:"properties>property,omitempty"`
}

type objectGroup struct {
	ID      int      `xml:"id,attr"`
	Name    string   `xml:"name,attr"`
	Objects []object `xml:"object"`
}

type tmxMap struct {
	XMLName      xml.Name     `xml:"map"`
	Version      string       `xml:"version,attr"`
	TiledVersion string       `xml:"tiledversion,attr"`
	Orientation  string       `xml:"orientation,attr"`
	RenderOrder  string       `xml:"renderorder,attr"`
	Width        int          `xml:"width,attr"`
	Height       int          `xml:"height,attr"`
	TileWidth    int          `xml:"tilewidth,attr"`
	TileHeight   int          `xml:"tileheight,attr"`
	Infinite     int          `xml:"infinite,attr"`
	NextLayerID  int          `xml:"nextlayerid,attr"`
	NextObjectID int          `xml:"nextobjectid,attr"`
	Properties   []property   `xml:"properties>property,omitempty"`
	Tileset      tileset      `xml:"tileset"`
	Layer        layer        `xml:"layer"`
	ObjectGroup  *objectGroup `xml:"objectgroup,omitempty"`
}

// Export は TMX を書き出す。地形は1枚のタイルレイヤー、ランドマークは
// オブジェクトレイヤーになる。グリッドの原点と1マスの度数はマップの
// カスタムプロパティとして残し、後から緯度経度へ戻せるようにする。
func Export(w io.Writer, g *worldgrid.Grid, placed []landmark.Placed, opts Options) error {
	opts = opts.withDefaults()

	doc := tmxMap{
		Version: "1.10", TiledVersion: "1.10.2",
		Orientation: "orthogonal", RenderOrder: "right-down",
		Width: g.Cols, Height: g.Rows,
		TileWidth: opts.TileSize, TileHeight: opts.TileSize,
		Infinite: 0, NextLayerID: 3, NextObjectID: len(placed) + 1,
		Properties: []property{
			{Name: "meshLevel", Type: "int", Value: strconv.Itoa(g.Level)},
			{Name: "originLat", Type: "float", Value: formatFloat(g.OriginLat)},
			{Name: "originLon", Type: "float", Value: formatFloat(g.OriginLon)},
			{Name: "latSpan", Type: "float", Value: formatFloat(g.LatSpan)},
			{Name: "lonSpan", Type: "float", Value: formatFloat(g.LonSpan)},
		},
		Tileset: buildTileset(opts),
		Layer: layer{
			ID: 1, Name: "terrain", Width: g.Cols, Height: g.Rows,
			Data: layerData{Encoding: "csv", CSV: encodeCSV(g)},
		},
	}
	if len(placed) > 0 {
		doc.ObjectGroup = buildObjects(placed, opts.TileSize)
	}

	if _, err := io.WriteString(w, xml.Header); err != nil {
		return err
	}
	enc := xml.NewEncoder(w)
	enc.Indent("", " ")
	if err := enc.Encode(doc); err != nil {
		return fmt.Errorf("TMXの書き出し: %w", err)
	}
	_, err := io.WriteString(w, "\n")
	return err
}

func buildTileset(opts Options) tileset {
	count := int(worldgrid.TerrainCount)
	tiles := make([]tile, 0, count)
	for t := worldgrid.Terrain(0); t < worldgrid.TerrainCount; t++ {
		tiles = append(tiles, tile{
			ID:   int(t),
			Type: t.String(),
			Properties: []property{
				{Name: "walkable", Type: "bool", Value: strconv.FormatBool(t.Walkable())},
			},
		})
	}
	return tileset{
		FirstGID: 1, Name: opts.TilesetName,
		TileWidth: opts.TileSize, TileHeight: opts.TileSize,
		TileCount: count, Columns: count,
		Image: tmxImage{
			Source: opts.TilesetImage,
			Width:  count * opts.TileSize, Height: opts.TileSize,
		},
		Tiles: tiles,
	}
}

// encodeCSV はタイルを Tiled の CSV レイヤー形式に並べる。
// GID は 0 が「空」を意味するため、地形の値に1を足す。
func encodeCSV(g *worldgrid.Grid) string {
	var sb strings.Builder
	sb.Grow(g.Cols*g.Rows*3 + g.Rows + 2)
	sb.WriteByte('\n')
	for row := 0; row < g.Rows; row++ {
		for col := 0; col < g.Cols; col++ {
			if col > 0 {
				sb.WriteByte(',')
			}
			sb.WriteString(strconv.Itoa(int(g.At(row, col)) + 1))
		}
		if row < g.Rows-1 {
			sb.WriteByte(',')
		}
		sb.WriteByte('\n')
	}
	return sb.String()
}

func buildObjects(placed []landmark.Placed, tileSize int) *objectGroup {
	objects := make([]object, 0, len(placed))
	for i, p := range placed {
		objects = append(objects, object{
			ID:   i + 1,
			Name: p.Name,
			Type: string(p.Kind),
			// Tiled のタイルオブジェクトではない矩形は左上基準。
			X: float64(p.Col * tileSize), Y: float64(p.Row * tileSize),
			Width: float64(tileSize), Height: float64(tileSize),
			Properties: []property{
				{Name: "id", Value: p.ID},
				{Name: "lat", Type: "float", Value: formatFloat(p.Lat)},
				{Name: "lon", Type: "float", Value: formatFloat(p.Lon)},
			},
		})
	}
	return &objectGroup{ID: 2, Name: "landmarks", Objects: objects}
}

func formatFloat(v float64) string {
	return strconv.FormatFloat(v, 'f', -1, 64)
}

// WriteTilesetPNG は地形1種につき1マスを横に並べたタイルセット画像を書き出す。
// TMX から参照される画像で、Tiled で開くために必要。
func WriteTilesetPNG(path string, tileSize int) error {
	if tileSize <= 0 {
		tileSize = 32
	}
	count := int(worldgrid.TerrainCount)
	img := image.NewRGBA(image.Rect(0, 0, count*tileSize, tileSize))
	for t := worldgrid.Terrain(0); t < worldgrid.TerrainCount; t++ {
		fill(img, int(t)*tileSize, tileSize, t.Color())
	}
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	defer f.Close()
	return png.Encode(f, img)
}

func fill(img *image.RGBA, originX, size int, c color.RGBA) {
	for y := 0; y < size; y++ {
		for x := 0; x < size; x++ {
			img.SetRGBA(originX+x, y, c)
		}
	}
}

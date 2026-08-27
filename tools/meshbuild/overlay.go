package main

import (
	"fmt"
	"os"
	"strings"

	"tokyo-quest-log/internal/geojson"
	"tokyo-quest-log/internal/worldgrid"
)

// overlayRequest は「この GeoJSON の形状を、この地形で塗る」という指示。
type overlayRequest struct {
	terrain worldgrid.Terrain
	path    string
}

// overlayList は -overlay を繰り返し指定できるようにする。
// 指定した順に適用されるため、重なりの優先順位は並び順で決まる。
type overlayList []overlayRequest

func (o *overlayList) String() string {
	specs := make([]string, len(*o))
	for i, req := range *o {
		specs[i] = fmt.Sprintf("%s=%s", req.terrain, req.path)
	}
	return strings.Join(specs, " ")
}

func (o *overlayList) Set(value string) error {
	name, path, ok := strings.Cut(value, "=")
	if !ok {
		return fmt.Errorf("地形名=パス の形式で指定してください: %q", value)
	}
	terrain, err := worldgrid.TerrainFromName(name)
	if err != nil {
		return err
	}
	if strings.TrimSpace(path) == "" {
		return fmt.Errorf("パスが空です: %q", value)
	}
	*o = append(*o, overlayRequest{terrain: terrain, path: path})
	return nil
}

func loadGeoJSON(path string) (geojson.Collection, error) {
	f, err := os.Open(path)
	if err != nil {
		return geojson.Collection{}, err
	}
	defer f.Close()
	c, err := geojson.Parse(f)
	if err != nil {
		return geojson.Collection{}, fmt.Errorf("%s: %w", path, err)
	}
	if c.Empty() {
		return geojson.Collection{}, fmt.Errorf("%s: 面も線も含まれていません", path)
	}
	return c, nil
}

// applyBoundary は境界の外側を都域外として塗る。
func applyBoundary(g *worldgrid.Grid, path string) (string, error) {
	c, err := loadGeoJSON(path)
	if err != nil {
		return "", err
	}
	if len(c.Polygons) == 0 {
		return "", fmt.Errorf("%s: 境界には面が必要です", path)
	}
	painted := g.MaskOutside(c.Polygons, worldgrid.OutOfArea)
	return fmt.Sprintf("  境界 %s: 外側 %d マスを都域外にしました（面 %d 個）",
		path, painted, len(c.Polygons)), nil
}

// applyOverlay は面を塗りつぶし、線を指定幅でなぞる。
// 内水面は陸地にだけ描き、海や都域外へはみ出さないようにする。
func applyOverlay(g *worldgrid.Grid, req overlayRequest, lineWidth int) (string, error) {
	c, err := loadGeoJSON(req.path)
	if err != nil {
		return "", err
	}
	onlyLand := req.terrain == worldgrid.Water
	filled := g.FillPolygons(c.Polygons, req.terrain)
	stroked := g.StrokeLines(c.Lines, lineWidth, req.terrain, onlyLand)

	var parts []string
	if len(c.Polygons) > 0 {
		parts = append(parts, fmt.Sprintf("面 %d 個 → %d マス", len(c.Polygons), filled))
	}
	if len(c.Lines) > 0 {
		parts = append(parts, fmt.Sprintf("線 %d 本（幅%d） → %d マス", len(c.Lines), lineWidth, stroked))
	}
	return fmt.Sprintf("  %s %s: %s", req.terrain, req.path, strings.Join(parts, ", ")), nil
}

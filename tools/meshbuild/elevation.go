package main

import (
	"encoding/csv"
	"fmt"
	"io"
	"os"
	"sort"
	"strconv"
	"strings"

	"tokyo-quest-log/internal/geomesh"
)

// elevationTable はメッシュコードから標高を引く表。
// 入力データの次数が混在しても扱えるよう、次数ごとに保持する。
type elevationTable struct {
	byLevel map[int]map[string]float64
	levels  []int // 細かい順
}

// loadElevationCSV は「メッシュコード,標高」形式のCSVを読み込む。
// 先頭行が数字で始まらない場合はヘッダとして読み飛ばす。
// 3列以上ある場合は2列目を標高として扱う。
func loadElevationCSV(path string) (*elevationTable, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	r := csv.NewReader(f)
	r.FieldsPerRecord = -1
	table := &elevationTable{byLevel: map[int]map[string]float64{}}

	for line := 1; ; line++ {
		record, err := r.Read()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, fmt.Errorf("%s:%d: %w", path, line, err)
		}
		if len(record) < 2 {
			return nil, fmt.Errorf("%s:%d: 列が足りない: %v", path, line, record)
		}
		code := strings.TrimSpace(record[0])
		if code == "" {
			continue
		}
		level, err := geomesh.LevelForCode(code)
		if err != nil {
			if line == 1 {
				continue // ヘッダ行
			}
			return nil, fmt.Errorf("%s:%d: %w", path, line, err)
		}
		elevation, err := strconv.ParseFloat(strings.TrimSpace(record[1]), 64)
		if err != nil {
			if line == 1 {
				continue // ヘッダ行
			}
			return nil, fmt.Errorf("%s:%d: 標高を数値として読めない: %q", path, line, record[1])
		}
		if table.byLevel[level] == nil {
			table.byLevel[level] = map[string]float64{}
		}
		table.byLevel[level][code] = elevation
	}

	for level := range table.byLevel {
		table.levels = append(table.levels, level)
	}
	sort.Sort(sort.Reverse(sort.IntSlice(table.levels)))
	if len(table.levels) == 0 {
		return nil, fmt.Errorf("%s: 有効なメッシュ行が1件もない", path)
	}
	return table, nil
}

// lookup は座標を含むメッシュの標高を返す。
// 入力に複数の次数が混在する場合は、細かい次数を優先する。
func (t *elevationTable) lookup(p geomesh.LatLon) (float64, bool) {
	for _, level := range t.levels {
		code, err := geomesh.Encode(p, level)
		if err != nil {
			continue
		}
		if elevation, ok := t.byLevel[level][code]; ok {
			return elevation, true
		}
	}
	return 0, false
}

func (t *elevationTable) summary() string {
	var sb strings.Builder
	for _, level := range t.levels {
		fmt.Fprintf(&sb, "  %d次メッシュ: %d件\n", level, len(t.byLevel[level]))
	}
	return sb.String()
}

package landmark

import (
	"os"
	"strings"
	"testing"

	"tokyo-quest-log/internal/worldgrid"
)

func TestLoadRejectsInvalid(t *testing.T) {
	cases := map[string]string{
		"空":       `{"landmarks":[]}`,
		"id重複":    `{"landmarks":[{"id":"a","name":"A","kind":"city","lat":35,"lon":139},{"id":"a","name":"B","kind":"town","lat":35,"lon":139}]}`,
		"id空":     `{"landmarks":[{"id":"","name":"A","kind":"city","lat":35,"lon":139}]}`,
		"name空":   `{"landmarks":[{"id":"a","name":"","kind":"city","lat":35,"lon":139}]}`,
		"未知のkind": `{"landmarks":[{"id":"a","name":"A","kind":"castle","lat":35,"lon":139}]}`,
		"壊れたJSON": `{"landmarks":`,
	}
	for name, doc := range cases {
		if _, err := Load(strings.NewReader(doc)); err == nil {
			t.Errorf("%s: エラーにならなかった", name)
		}
	}
}

// 同梱のランドマークデータが読め、すべて都域のグリッドに収まること。
func TestBundledLandmarksFitInGrid(t *testing.T) {
	f, err := os.Open("../../assets/landmarks.json")
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	landmarks, err := Load(f)
	if err != nil {
		t.Fatal(err)
	}
	if len(landmarks) < 20 {
		t.Errorf("ランドマーク数 = %d, 20件以上を期待", len(landmarks))
	}

	g, err := worldgrid.New(worldgrid.TokyoMainland, 5)
	if err != nil {
		t.Fatal(err)
	}
	placed, skipped := Place(g, landmarks)
	if len(skipped) > 0 {
		t.Errorf("グリッド外のランドマーク: %v", skipped)
	}
	if len(placed) != len(landmarks) {
		t.Fatalf("配置数 %d, 読み込み数 %d", len(placed), len(landmarks))
	}

	// 同じマスに複数が重なっていないか確認する。重なると片方が引けなくなる。
	idx := NewIndex(placed)
	if len(idx) != len(placed) {
		t.Errorf("マスの重複がある: 索引 %d 件に対し配置 %d 件", len(idx), len(placed))
	}
}

// 東西南北の位置関係が保たれること。緯度経度からマスへの変換の向きを確かめる。
func TestPlaceOrientation(t *testing.T) {
	g, err := worldgrid.New(worldgrid.TokyoMainland, 5)
	if err != nil {
		t.Fatal(err)
	}
	marks := []Landmark{
		{ID: "west", Name: "八王子", Kind: KindCity, Lat: 35.656, Lon: 139.339},
		{ID: "east", Name: "葛西臨海", Kind: KindPort, Lat: 35.641, Lon: 139.862},
		{ID: "north", Name: "奥多摩", Kind: KindTown, Lat: 35.809, Lon: 139.096},
		{ID: "south", Name: "羽田", Kind: KindPort, Lat: 35.549, Lon: 139.780},
	}
	placed, skipped := Place(g, marks)
	if len(skipped) > 0 {
		t.Fatalf("配置できなかった: %v", skipped)
	}
	byID := map[string]Placed{}
	for _, p := range placed {
		byID[p.ID] = p
	}
	if byID["west"].Col >= byID["east"].Col {
		t.Errorf("西の地点が東より右にある: %d vs %d", byID["west"].Col, byID["east"].Col)
	}
	if byID["north"].Row >= byID["south"].Row {
		t.Errorf("北の地点が南より下にある: %d vs %d", byID["north"].Row, byID["south"].Row)
	}
}

func TestPlaceSkipsOutOfRange(t *testing.T) {
	g, err := worldgrid.New(worldgrid.TokyoMainland, 5)
	if err != nil {
		t.Fatal(err)
	}
	_, skipped := Place(g, []Landmark{{ID: "osaka", Name: "大阪", Kind: KindCity, Lat: 34.702, Lon: 135.495}})
	if len(skipped) != 1 {
		t.Errorf("範囲外が除外されていない: %v", skipped)
	}
}

func TestMeshCode(t *testing.T) {
	l := Landmark{ID: "tokyo", Name: "東京", Kind: KindCity, Lat: 35.681, Lon: 139.767}
	code, err := l.MeshCode()
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(code, "53394611") {
		t.Errorf("東京のメッシュコード = %q, 53394611 で始まることを期待", code)
	}
}

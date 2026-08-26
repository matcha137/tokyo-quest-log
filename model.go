package main

import "math"

type Layer int

const (
	LayerMovement Layer = iota
	LayerLife
	LayerTerrain
)

type Point struct {
	X float64
	Y float64
}

type MapFeature struct {
	Layer  Layer
	Kind   string
	Label  string
	Points []Point
}

type Quest struct {
	ID          string
	Title       string
	ShortTitle  string
	Layer       Layer
	Position    Point
	Description []string
	Question    string
	Choices     []string
	Correct     int
	Discovery   []string
	Source      string
	Completed   bool
}

type World struct {
	Player       Point
	Quests       []Quest
	Features     []MapFeature
	ActiveQuest  int
	Feedback     string
	ShowComplete bool
}

func NewWorld() *World {
	return &World{
		Player:      Point{X: 430, Y: 500},
		ActiveQuest: -1,
		Quests: []Quest{
			{
				ID:         "gateway",
				Title:      "街の入口を読み解け",
				ShortTitle: "街の入口",
				Layer:      LayerMovement,
				Position:   Point{X: 690, Y: 455},
				Description: []string{
					"駅と道路は、街へ入るための最初の手がかり。",
					"現在地から文化施設へ向かう経路を考えよう。",
				},
				Question: "初めての街で移動の基準点にしやすいものは？",
				Choices:  []string{"1. 駅と主要道路", "2. 建物の屋根の色", "3. 店の混雑だけ"},
				Correct:  0,
				Discovery: []string{
					"移動レイヤーを復元した。",
					"駅、主要道路、街のつながりが地図に加わった。",
				},
				Source: "公共交通・道路オープンデータを想定",
			},
			{
				ID:         "place",
				Title:      "休日の居場所を探せ",
				ShortTitle: "休日の居場所",
				Layer:      LayerLife,
				Position:   Point{X: 330, Y: 250},
				Description: []string{
					"知らない街にも、無料で立ち寄れる場所がある。",
					"条件に合う公共施設を見つけよう。",
				},
				Question: "雨の日も無料で過ごしやすい公共施設は？",
				Choices:  []string{"1. 屋外広場", "2. 図書館", "3. 駐車場"},
				Correct:  1,
				Discovery: []string{
					"生活レイヤーを復元した。",
					"図書館、公園、文化施設が地図に加わった。",
				},
				Source: "公共施設・公園オープンデータを想定",
			},
			{
				ID:         "water",
				Title:      "水辺の記憶をたどれ",
				ShortTitle: "水辺の記憶",
				Layer:      LayerTerrain,
				Position:   Point{X: 625, Y: 205},
				Description: []string{
					"池、坂、低地は街の形をつくっている。",
					"水辺と周囲の高低差を地図から推理しよう。",
				},
				Question: "水が集まりやすい地形として考えやすいのは？",
				Choices:  []string{"1. 尾根", "2. 台地の頂部", "3. 周囲より低い土地"},
				Correct:  2,
				Discovery: []string{
					"地形レイヤーを復元した。",
					"池、坂、高低差が地図に加わった。",
				},
				Source: "標高・河川オープンデータを想定",
			},
		},
		Features: sampleFeatures(),
	}
}

func sampleFeatures() []MapFeature {
	return []MapFeature{
		{Layer: LayerMovement, Kind: "road", Label: "ことぶき通り", Points: []Point{{90, 510}, {230, 445}, {420, 465}, {610, 420}, {795, 455}}},
		{Layer: LayerMovement, Kind: "road", Label: "丘の道", Points: []Point{{190, 120}, {250, 225}, {330, 310}, {430, 500}}},
		{Layer: LayerMovement, Kind: "road", Label: "水辺通り", Points: []Point{{520, 105}, {570, 210}, {650, 315}, {690, 455}}},
		{Layer: LayerMovement, Kind: "station", Label: "上野口駅", Points: []Point{{690, 455}}},

		{Layer: LayerLife, Kind: "library", Label: "まちの図書室", Points: []Point{{330, 250}}},
		{Layer: LayerLife, Kind: "park", Label: "谷中の森", Points: []Point{{205, 165}}},
		{Layer: LayerLife, Kind: "museum", Label: "文化館", Points: []Point{{485, 330}}},
		{Layer: LayerLife, Kind: "rest", Label: "休憩広場", Points: []Point{{235, 440}}},

		{Layer: LayerTerrain, Kind: "water", Label: "不忍の池", Points: []Point{{565, 145}, {640, 125}, {705, 175}, {680, 245}, {590, 255}, {545, 205}}},
		{Layer: LayerTerrain, Kind: "contour", Label: "台地", Points: []Point{{105, 155}, {170, 115}, {265, 130}, {320, 205}, {300, 300}, {205, 350}, {125, 300}}},
		{Layer: LayerTerrain, Kind: "slope", Label: "三段坂", Points: []Point{{310, 330}, {355, 380}, {410, 405}}},
	}
}

func (w *World) CompletedCount() int {
	count := 0
	for _, q := range w.Quests {
		if q.Completed {
			count++
		}
	}
	return count
}

func (w *World) LayerUnlocked(layer Layer) bool {
	for _, q := range w.Quests {
		if q.Layer == layer && q.Completed {
			return true
		}
	}
	return false
}

func (w *World) NearbyQuest(radius float64) int {
	for i := range w.Quests {
		q := &w.Quests[i]
		if q.Completed {
			continue
		}
		if math.Hypot(w.Player.X-q.Position.X, w.Player.Y-q.Position.Y) <= radius {
			return i
		}
	}
	return -1
}

func (w *World) OpenNearbyQuest(radius float64) bool {
	idx := w.NearbyQuest(radius)
	if idx < 0 {
		return false
	}
	w.ActiveQuest = idx
	w.Feedback = ""
	return true
}

func (w *World) Answer(choice int) bool {
	if w.ActiveQuest < 0 || w.ActiveQuest >= len(w.Quests) {
		return false
	}
	q := &w.Quests[w.ActiveQuest]
	if choice != q.Correct {
		w.Feedback = "まだ地図は復元されない。街の特徴から、もう一度考えよう。"
		return false
	}
	q.Completed = true
	w.Feedback = ""
	w.ActiveQuest = -1
	if w.CompletedCount() == len(w.Quests) {
		w.ShowComplete = true
	}
	return true
}

func (w *World) Reset() {
	reset := NewWorld()
	*w = *reset
}

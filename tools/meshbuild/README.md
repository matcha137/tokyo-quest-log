# meshbuild

標高メッシュのCSVから、東京都のワールドマップ用タイルグリッドを生成する。

```
go run ./tools/meshbuild -in data/elevation.csv -out assets/tokyo_mainland.bin -preview preview.png
```

## 入力形式

`メッシュコード,標高(m)` の2列のCSV。先頭行がヘッダでも構わない。

```csv
meshcode,elevation
53394611,3.2
53394612,4.1
```

- 3次（8桁 / 約1km）〜5次（10桁 / 約250m）のメッシュコードを受け付ける
- 次数が混在していても動作し、細かい次数を優先して参照する
- 3列以上ある場合は2列目を標高として扱う

## 出力するグリッド

1マスが標準地域メッシュの1区画に対応する。既定の5次メッシュでは
**南北 約232m × 東西 約283m**（正方形ではない）。

東京都本土（既定の `-bounds`）で **324 × 202 = 65,448マス、64KB**。
`go:embed` でそのまま実行バイナリに埋め込める大きさ。

## 水域と境界の重ね合わせ

標高だけでは、内陸の河川も、隣県との境も表現できない。GeoJSON を重ねて補う。

```
go run ./tools/meshbuild -in data/elevation.csv   -boundary data/tokyo_boundary.geojson   -overlay water=data/rivers.geojson   -line-width 2 -preview preview.png
```

適用順は **境界 → -overlay の指定順** で固定されている。境界を先に確定させることで、
河川が都域外へはみ出して描かれるのを防ぐ。

- **面**（Polygon / MultiPolygon）は内側を塗りつぶす。2番目以降の輪は穴として抜ける
- **線**（LineString / MultiLineString）は `-line-width` マスの幅でなぞる
- `water`（内水面）を塗るときだけ、海と都域外のマスは塗り残す

`-boundary` を指定しない場合、都域外は生成されない。標高データが東京都のみを
収録しているなら不要だが、国土地理院の標高タイルのように全国を覆うデータでは
指定しないと隣県が陸地として残る。

### 地形名

`-overlay` の地形名には次が使える（英語名・日本語名のどちらでも可）。

`sea`/`海`, `lowland`/`低地`, `plain`/`平地`, `plateau`/`台地`, `hill`/`丘陵`,
`mountain`/`山地`, `highmountain`/`高山`, `water`/`内水面`, `outofarea`/`都域外`

## 地形の決まり方

標高の帯で機械的に振り分ける（`classify.go` の `defaultBands`）。

| 地形 | 標高 |
|---|---|
| 低地 | 〜5m |
| 平地 | 5〜50m |
| 台地 | 50〜150m |
| 丘陵 | 150〜400m |
| 山地 | 400〜900m |
| 高山 | 900m〜 |
| 海 | 標高データなし |
| 内水面 | 重ね合わせで指定（河川・湖沼） |
| 都域外 | `-boundary` の外側 |

**海は「標高0以下」ではなく「データが存在しない」で判定する。** 国土数値情報の
標高メッシュは陸域のみを収録するため、欠損がそのまま海岸線になる。江東5区などの
海抜0m地帯は標高データを持つので、正しく陸地として残る。

## 主なフラグ

| フラグ | 既定値 | 意味 |
|---|---|---|
| `-in` | （必須） | 標高メッシュCSV |
| `-out` | `assets/tokyo_mainland.bin` | グリッドの出力先 |
| `-preview` | なし | 確認用PNGの出力先 |
| `-level` | `5` | 出力メッシュ次数（5=250m, 4=500m, 3=1km） |
| `-scale` | `3` | プレビュー1マスあたりのピクセル数 |
| `-bounds` | 東京都本土 | `minLat,minLon,maxLat,maxLon` |
| `-boundary` | なし | 都域境界のGeoJSON。外側を都域外にする |
| `-overlay` | なし | `地形名=GeoJSONのパス`。繰り返し指定可 |
| `-line-width` | `1` | 線状の水域をなぞる幅（マス） |
| `-landmarks` | なし | ランドマークJSON。繰り返し指定可（同じマスは先勝ち） |
| `-tmx` | なし | Tiled形式(.tmx)の出力先 |
| `-tile-size` | `32` | TMXの1マスのピクセル数 |

島嶼部は本土から遥かに離れるため、`-bounds` で別途切り出して別マップとして扱う。

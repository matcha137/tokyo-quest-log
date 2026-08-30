# osmtown

OpenStreetMap のデータから、街を歩く詳細マップを作る。

```
go run ./tools/osmtown -in data/osm/tokyo_station.json \
  -bounds 35.6700,139.7480,35.6950,139.7830 \
  -out assets/town_tokyo.bin -preview town.png
```

## なぜ標高を使わないのか

ワールドマップは標高から地形を決めているが、街のスケールでは効かない。
東京駅周辺2km四方の高低差は **14.9m**（最小2.4m / 中央4.0m / 最大17.3m）しかなく、
標高帯で分けるとほぼ全面が同じ色になる。

街の骨格は道路・建物・線路・水面・緑地なので、それを地形として扱う。

## データの取り方

Overpass API に問い合わせて JSON で受け取る。`out geom` でジオメトリを
埋め込ませることで、ノード参照の解決が不要になる。

```
[out:json][timeout:90];
(
  way["highway"](35.6700,139.7480,35.6950,139.7830);
  way["building"](35.6700,139.7480,35.6950,139.7830);
  way["railway"](35.6700,139.7480,35.6950,139.7830);
  way["waterway"](35.6700,139.7480,35.6950,139.7830);
  way["natural"="water"](35.6700,139.7480,35.6950,139.7830);
  way["leisure"="park"](35.6700,139.7480,35.6950,139.7830);
);
out geom;
```

```
curl -s -A "..." --data-urlencode "data@town.ql" \
  -o data/osm/tokyo_station.json https://overpass-api.de/api/interpreter
```

**`-bounds` は必ず指定すること。** Overpass は範囲に掛かった way を丸ごと返すため、
線路や幹線道路が範囲外まで伸びる。指定しないとデータの外接矩形が使われ、
意図の数倍の広さになる（東京駅の例では 3.2km 四方のつもりが 11.9km になった）。

## 塗る順番

後の層ほど上に載る。建物を最後にすることで街区の輪郭が残る。

```
地面（既定） → 公園 → 水面 → 線路 → 道路 → 建物
```

閉じた way（始点と終点が一致）は面として塗りつぶし、それ以外は線としてなぞる。
線の幅は種別ごとの実寸から決める（幹線30m、一般12m、歩道4m など）。

## 地下鉄の除外

都心は地下鉄が縦横に走っており、そのまま壁として塗ると実際には存在しない
障害物で街が分断される。次のものは線路として塗らない。

- `railway=subway` / `platform` / `abandoned` / `construction` など
- `tunnel` が設定されているもの
- `layer` が負のもの

## 主なフラグ

| フラグ | 既定値 | 意味 |
|---|---|---|
| `-in` | （必須） | Overpass API の JSON |
| `-bounds` | データの外接矩形 | `minLat,minLon,maxLat,maxLon` |
| `-out` | `assets/town_tokyo.bin` | グリッドの出力先 |
| `-preview` | なし | 確認用PNG |
| `-level` | `10` | メッシュ次数（10で東京付近およそ7m × 9m） |
| `-scale` | `2` | プレビュー1マスあたりのピクセル数 |

## 既知の制約

- **リレーションを読まない。** 一部の大きな公園や建物はリレーションで表現されており、
  それらは落ちる。way だけで実用上は足りている
- 高架・地下の道路を区別していない。地上として塗る

## 出典

OpenStreetMap contributors、**ODbL**。生成物 `assets/town_tokyo.bin` は
OSM データの派生データベースにあたるため、配布する場合は ODbL の
継承条件が及ぶ。

# 常駐キャラクター（chara）

待機画面に自分のキャラクターを常駐させ、会話をチャット風の画面にする機能です。Muse のドット絵スプライト（`tools/muse-avatar`）とは別物で、会話中だけでなく一日中画面にいます。絵は自分で用意し、端末に置きます。ソースにもリリースにも絵は含みません。

## 置くもの

端末の `/data/misc/techo5/chara/` に次のファイルを置きます。更新で消えない場所です。

| ファイル | 必須 | 内容 |
| --- | --- | --- |
| `idle.png` | 必須 | 立ち絵。背景透過。縦長（たとえば 600×900）。待機画面では画面の高さの 86% に縮小 |
| `busy.png` | 任意 | 作業中の絵（ノート PC を開いているなど）。会話の「作業中」カードに出る。無ければ `idle.png` |
| `background.png` | 任意 | 部屋の絵。画面いっぱい（960×480 以上の横長）に切り抜いて敷く。無ければすりガラスの背景 |
| `rooms/<時間帯>_<天気>.png` | 任意 | 時間帯と天気ごとの部屋の絵。あれば `background.png` の代わりに使う（下の「部屋の時間帯と天気」） |
| `pose_<名前>.png` | 任意 | 部屋で過ごすポーズ（机で勉強、コーヒーなど）。背景透過。`chara.json` の `poses` で置き場所を決める |
| `chara.json` | 任意 | 名前と吹き出しの台詞、天気の地名 |

```json
{
  "name": "Sarah",
  "place": "東京",
  "lines": ["ねえ、何か用事ある？", "今日もいい日になりますように"]
}
```

- `name` は会話画面の見出しと「〜が作業中」に使います。省略すると `Muse`
- `lines` は待機画面の吹き出しに 3 分ごとに順番に出ます。空なら吹き出しは出ません
- `place` は天気タイルの「今日・東京」の地名。天気の場所を設定していればそちらを使い（「Saitama, Japan」なら「Saitama」）、`place` は設定が無いときだけ使います。日本語の地名にしたいときは、画面の言語を日本語にしてからセットアップページで場所を登録し直します（「さいたま市, 埼玉県」で登録されます）

### 表情差分（任意）

同じ構図のまま顔だけ変えた絵を置くと、キャラが動きます。codex などの画像編集で「ポーズ・構図・透過はそのまま、目だけ閉じて」と頼んで作ります。

| ファイル | 元 | 変えるところ | 使いどころ |
| --- | --- | --- | --- |
| `idle_eyes_half.png` | idle | まぶたを半分 | 瞬きの途中 |
| `idle_eyes_closed.png` | idle | 目を閉じる | 瞬き |
| `idle_happy.png` | idle | にっこり（^ ^）、口を少し開く | なでられたとき |
| `busy_eyes_closed.png` | busy | 目を閉じる | 作業中の瞬き |
| `busy_lookup.png` | busy | 右上を見上げる、口は「むー」 | 考え中 |

読み込み時に元の絵との差分を取り、顔の範囲だけを切り出して重ねます。顔の外まで変わっている絵（構図がずれた絵）は捨て、ログに `character expression dropped` と出ます。差分が無ければ、その動きだけ無しになります（半目が無ければ閉じ目だけで瞬き、閉じ目が無ければ瞬きなし）。

`chara.json` の `petted` は、なでられたときの吹き出しの台詞です。

### 部屋の時間帯と天気（任意）

`rooms/` に同じ部屋の明かりと窓の外だけを変えた絵を置くと、時間と天気で部屋が切り替わります。名前は `<時間帯>_<天気>.png` です。

| 時間帯 | いつ |
| --- | --- |
| `dawn` | 日の出の 1 時間前から 30 分後まで |
| `morning` | それから 10 時まで |
| `day` | 日の入りの 1 時間前まで |
| `evening` | 日の入りの 40 分後まで |
| `night` | それ以外 |

日の出と日の入りは天気の場所の緯度経度から計算します（場所が分かるまでは 6:00 と 17:30）。天気は `clear`（晴れ・晴れ時々曇り）、`cloudy`（曇り・霧・雪）、`rain`（雨・雷雨・ひょう）です。無い組み合わせはその時間帯の `clear`、それも無ければ `background.png` を使います。照明をつけるかどうかは絵に描き込みます。

全部を展開すると 1 枚 6 MB になるので、使う 1 枚だけを画面の大きさに縮めて持ちます。切り替わるときは描画とは別に読み込み、終わるまでは前の部屋のままです。

### 部屋で過ごすポーズ（任意）

部屋の家具の位置に合わせて、座る・くつろぐなどのポーズを置けます。絵は部屋と同じ縮尺で描かれ、家具の上に重なります。

```json
"poses": [
  {"name": "study",  "x": 1255, "y": 807, "h": 530, "hours": ["morning", "day", "night"], "lines": ["いま勉強中…"]},
  {"name": "coffee", "x": 1255, "y": 804, "h": 522, "hours": ["dawn", "morning", "evening"]},
  {"name": "relax",  "x": 970,  "y": 985, "h": 535, "hours": ["evening", "night"]}
]
```

- `name` は絵の名前で、`pose_study.png` を読みます。`pose_study_eyes_closed.png` があれば瞬きします
- `x` と `y` は絵の下辺の真ん中、`h` は絵の高さで、部屋の絵（`background.png` か `rooms/` の絵）の画素で書きます。座るポーズは太ももの裏（絵の高さの下から 1/4 あたり）が座面に来るように、くつろぐポーズはクッションのくぼみに腰が沈むように合わせると自然です。下辺は画面の外に出てもかまいません
- `hours` はそのポーズをする時間帯（上の表）。省略するといつでも
- `lines` はそのポーズの間の吹き出しの台詞。省略すると `lines`

30 分ごとに、時間帯に合うポーズと立ち姿の中から 1 つを選びます。同じ 30 分の間は再起動しても同じです。時計やタイルに重なるポーズはそのレイアウトでは選びません（いつもの: 左 610 px、時計大: 左 290 px。キャラ大はいつも立ち姿）。なでる・呼びかける・会話すると手前の立ち姿に戻り、3 分たつと部屋に戻ります。切り替わりは 0.3 秒のクロスフェードです。

### 動き

- **瞬き**: 2 秒 + 指数分布（平均 2 秒、最長 8 秒）の間隔で、半目 80 ms・閉じ 80 ms・半目 80 ms。15% で 2 回連続。吹き出しの交代やフェーズの切り替わりでも瞬きます
- **呼吸**: 昼だけ。4.2 秒周期（吸う 1.6、吐く 2.2、止め 0.4）で、足元を固定して最大 0.8% 縦に伸びます。6 コマ/秒
- **夜（夜間モード中）**: 呼吸なし、瞬きは 6〜12 秒間隔
- **なでる**: キャラをタップすると、縮んで伸びて着地（約 0.3 秒）、1.2 秒にっこり、頭の上にキラが 3 つ昇って消え、吹き出しが `petted` の台詞に替わります。連打は 1.5 秒に 1 回まで
- **会話**: 聞き取り開始で 3 px、返答開始で 2 px 跳ねます。作業中カードでは書く顔 2.5〜4 秒と見上げ顔 1.2 秒を交互に、頭の横に「…」の思考吹き出し（文字の「…」と同じ 400 ms 刻み）

待機画面の家具（部屋・時計・タイル・ボタン）は 1 分に 1 回しか描き直さず、毎コマはそのコピーにキャラと吹き出しを重ねるだけなので、6〜12 コマ/秒でも負荷は小さいです。

置いたら `killall techo5` で読み直します。ログに `resident character loaded` と出れば有効です。壊れた PNG や 1600 万画素を超える絵は読まず、`resident character not loaded` と理由を出します。

## 画面

- **待機画面**: 部屋を背景に、右にキャラ、左に大きな時計と日付、今日と明日の天気のガラスタイル、「話す・ラジオ・天気・設定」のボタン、「Ready · Hey Muse」のピル。タイマーが動いていれば残り時間がピルの右に出ます。マイクを切ると赤い点と「マイクオフ」になります
- **会話画面**: 上に「‹ 待機画面」、キャラの小さな顔と名前と状態（聞いています・作業中・話しています・オンライン）、時計。聞き取った言葉は右の緑の吹き出し、返事は左の白い吹き出し。返事が来るまでは `busy.png` と「〜が作業中 / 回答を準備しています…」のカード。長い返事は「前へ・次へ」でページを送ります。足元の帯は「タップで会話を中断」、返事が残っている間は「タップして話す」
- **起動画面**: Hey Muse のロゴの上にキャラが出ます

待機画面のタップは、ボタンはそれぞれの動作（話す = 会話開始、ラジオ = ラジオの引き出し、天気 = 予報ページ、設定 = 設定シート）、天気タイルは予報ページです。それ以外の場所は従来どおり会話を始めます。

## 作りの要点

- `echod/internal/feature/display/chara.go`: 読み込みと縮小（`golang.org/x/image/draw` の CatmullRom）。画面サイズごとに縮小結果を持ち、Show 8 でも同じ割合で出ます
- `echod/internal/feature/display/render_chara.go`: 3 画面の描画。座標は Show 5 の画素で書き、`r.s` で拡大します
- `render.go` の `draw` が、キャラがあれば会話を `drawCharaTurn`、待機を `drawCharaHome` に回します。SwitchBot の待機画面（`idle-home.json`）より優先します
- `echod/internal/feature/display/chara_room.go`: 時間帯と天気から部屋の絵を選ぶ
- `echod/internal/feature/display/chara_pose.go`: ポーズの読み込み、部屋の座標から画面への配置、30 分ごとの選択
- プレビューは `SHOW_PREVIEW=<dir> go test ./internal/feature/display -run TestCharaScenesDraw`。端末の絵で全部の部屋を描くには `CHARA_DIR=/data/misc/techo5/chara SHOW_PREVIEW=<dir>` で `-run TestCharaRoomsPreview`（Linux で。macOS では display パッケージのテストが bluez の都合で走りません）

## 端末に残す

bind-mount の試験投入は再起動で消えます。残すときはバージョンを付けてビルドし、起動中のスロットの本体を置き換えます。バージョンを付けないと `v0.0.0-dev` になり、updater が公式リリースを「新しい版」と見て入れ直してしまいます。公式より新しく見える名前（公式が `v1.2.0-muse.3` なら `v1.2.0-muse.3.pom.1` のように末尾を伸ばす）にすると、更新は提示されません。

```sh
pkg=github.com/HuskerMinion/techo5/echod/internal/layout
cd echod && GOOS=linux GOARCH=arm GOARM=7 CGO_ENABLED=0 go build -trimpath \
  -ldflags "-s -w -X $pkg.Version=v1.2.0-muse.3.pom.1 -X $pkg.GitCommit=$(git rev-parse --short HEAD) -X $pkg.BuildDate=$(date -u +%Y-%m-%dT%H:%M:%SZ)" \
  -o ../bin/echod-arm ./cmd/echod
scp -O ../bin/echod-arm root@<address>:/tmp/echod-new
ssh root@<address> 'S=/usr/local/sbin/slotctl; slot=$(cat /run/techo5/slot); $S rw &&
  cp /tmp/echod-new /store/slots/$slot/usr/local/bin/techo5.new && mv -f /store/slots/$slot/usr/local/bin/techo5.new /store/slots/$slot/usr/local/bin/techo5 &&
  sync; mount -o remount,ro /store; rm -f /tmp/echod-new; killall techo5'
```

`mount -o remount,ro` が busy で失敗しても、次の再起動で読み取り専用に戻ります。もう一方のスロットは触らないので、戻したいときは `slotctl switch` で公式のままの側を起動します。

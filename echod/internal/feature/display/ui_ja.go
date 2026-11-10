//go:build !dot && !spot

package display

import "strings"

// The settings screen in Japanese. The screen's own words are English where they are written, and
// this is a table they are looked up in as they are drawn, when the screen's language is Japanese.
// A word missing from the table is drawn in English, so a new setting arriving from upstream shows
// up untranslated rather than blank.

// ui is s in the screen's language: the Japanese from uiJa when the screen is Japanese and the table
// has it, s itself otherwise.
func ui(s string) string {
	if s == "" || screenLang() != "ja" {
		return s
	}
	if t, ok := uiJa[s]; ok {
		return t
	}
	for _, p := range uiJaPrefix {
		if rest, ok := strings.CutPrefix(s, p.en); ok {
			return p.ja(rest)
		}
	}
	return s
}

// uiRow is a settings row with its words in the screen's language.
func uiRow(row settingRow) settingRow {
	row.label, row.sub, row.value, row.button = ui(row.label), ui(row.sub), ui(row.value), ui(row.button)
	return row
}

// uiAll is a list of choices in the screen's language.
func uiAll(opts []string) []string {
	if screenLang() != "ja" {
		return opts
	}
	out := make([]string, len(opts))
	for i, o := range opts {
		out[i] = ui(o)
	}
	return out
}

// uiJaPrefix are the words that come with something of the device's own after them: an address, a
// name, a time.
var uiJaPrefix = []struct {
	en string
	ja func(rest string) string
}{
	{"Open ", func(rest string) string { return rest + " を開く" }},
	{"Snoozed until ", func(rest string) string { return rest + " までスヌーズ" }},
	{"From Home Assistant · ", func(rest string) string { return "Home Assistant から · " + rest }},
	{"Forget ", func(rest string) string { return rest + " を削除" }},
}

var uiJa = map[string]string{
	// The rail, the card headings and what is under them.
	"Display":            "画面",
	"Sound":              "サウンド",
	"Alarms":             "アラーム",
	"Connections":        "接続",
	"Privacy":            "プライバシー",
	"General":            "一般",
	"Sound & Voice":      "サウンドと音声",
	"Alarms & Timers":    "アラームとタイマー",
	"Privacy & Security": "プライバシーとセキュリティ",
	"Brightness, night, theme, home screen and weather": "明るさ・夜間・テーマ・ホーム画面・天気",
	"Volume, voice, quiet hours and music":              "音量・音声・おやすみ時間・音楽",
	"Alarms on this device and how they ring":           "この端末のアラームと鳴り方",
	"Wi-Fi, Bluetooth and the Bluetooth proxy":          "Wi-Fi・Bluetooth・Bluetooth プロキシ",
	"Remote access and how this device is reached":      "リモート接続と、この端末への接続方法",
	"Name, weather, updates and restart":                "名前・天気・アップデート・再起動",

	// Buttons and the values settings show.
	"Done": "完了", "Save": "保存", "Cancel": "キャンセル", "Edit": "編集", "Delete": "削除", "Set": "設定",
	"Change": "変更", "Confirm": "確認", "Install": "インストール", "Check now": "今すぐ確認",
	"Check for updates": "アップデートを確認", "Connect": "接続", "Disconnect": "切断", "Pair": "ペアリング",
	"Restart": "再起動", "Forget": "削除", "‹  Back": "‹  戻る", "+ Add alarm": "+ アラームを追加",
	"On": "オン", "Off": "オフ", "Never": "なし", "None": "なし", "None yet": "まだありません",
	"Muted": "ミュート", "Closed": "閉じる", "Encrypted": "暗号化", "Not encrypted": "暗号化なし",
	"Not connected": "未接続", "No login": "ログインなし", "Running": "動作中", "Silent": "無音",
	"Set there": "そちらで設定", "Not managed here": "ここでは管理しません",
	"Not available on this build": "このビルドでは使えません", "Back in about a minute": "1 分ほどで戻ります",
	"Asks twice": "2 回確認", "Pop up": "ポップアップ", "Home": "ホーム", "Timer": "タイマー",

	// Display.
	"Brightness": "明るさ", "Auto-brightness": "明るさの自動調整", "Follows the room's light": "部屋の明るさに合わせる",
	"Dimmest": "最低の明るさ", "How dark auto-brightness goes in a dark room": "暗い部屋で自動調整がどこまで暗くするか",
	"Night hours": "夜間の時間帯", "Night starts": "夜間の開始", "Night ends": "夜間の終了",
	"At night": "夜間", "Night clock": "夜の時計", "How the night clock looks": "夜の時計の見た目",
	"Dark, a faint glow, or a clock alone until touched": "真っ暗、ほのかな光、または触るまで時計だけ",
	"Look": "見た目", "Theme": "テーマ", "Custom colors": "カスタムカラー", "Make the theme your own": "テーマを自分好みに",
	"Tap a color for each part; the theme becomes Custom": "部分ごとに色をタップ。テーマはカスタムになります",
	"Background":  "背景",
	"Home screen": "ホーム画面", "Clock style": "時計のスタイル", "How the clock looks all day": "日中の時計の見た目",
	"Clock position": "時計の位置", "Out of a photo's way": "写真の邪魔にならない位置",
	"Date color": "日付の色", "The date and AM/PM": "日付と午前/午後", "Clock format": "時刻の表示形式",
	"Tap on the clock": "時計をタップしたとき",
	"Start Assist, open the dashboard or the deck, or nothing": "Assist を開始、ダッシュボードかデッキを開く、または何もしない",
	"Screen language": "画面の言語", "Turn screen": "画面の向き", "Screensaver": "スクリーンセーバー",
	"Slideshow": "スライドショー", "Time per photo": "1 枚の表示時間", "Photo folder": "写真のフォルダ",
	"Photos from Home Assistant": "Home Assistant の写真", "Include subfolders": "サブフォルダも含める",
	"Every folder inside the one chosen": "選んだフォルダの中のすべてのフォルダ",
	"Shuffle photos":                     "写真をシャッフル", "Show whole photo": "写真全体を表示",
	"All of it, with blurred sides; off fills the screen": "左右をぼかして全体を表示。オフで画面いっぱい",
	"Weather art": "天気の絵", "A landscape for the weather, in place of photos": "写真の代わりに天気の風景",
	"Weather": "天気", "Weather animation": "天気のアニメーション",
	"Rain, snow and storms move on the forecast": "予報ページで雨・雪・嵐が動く",
	"Weather alerts": "気象警報", "The NWS's alerts for home, in the U.S.": "米国 NWS の警報（米国のみ）",
	"Radar source": "雨雲レーダーの提供元", "Automatic uses the NWS in the lower 48": "自動は米本土で NWS を使う",
	"Now playing": "再生中の表示", "Full page, or a strip over the clock": "全画面、または時計の上の帯",
	"Now playing follows": "再生中の表示を追従", "Lyrics": "歌詞",
	"The words in time, looked up at LRCLIB": "LRCLIB で調べた歌詞を曲に合わせて表示",
	"Answer time":                            "返答の表示時間", "How long an answer stays up; a tap clears it": "返答を表示しておく時間。タップで消える",
	"Presence": "人感", "Presence detection": "人の検知",
	"Screen off when nobody is near":                    "近くに誰もいないとき画面を消す",
	"Lights again when somebody comes near":             "誰かが近づくとまた点く",
	"The camera notices somebody near; nothing is kept": "カメラで近くの人を検知。映像は保存しません",

	// Sound & Voice.
	"Volume": "音量", "Bass": "低音", "Treble": "高音", "Voice": "音声", "Microphone": "マイク",
	"The mute button does this too": "ミュートボタンでも切り替わります",
	"Wake word":                     "ウェイクワード", "Wake word sensitivity": "ウェイクワードの感度",
	"Higher wakes by mistake less often": "高いほど誤反応が減る",
	"Wake sound":                         "起動音", "Speaking voice": "話す声", "How answers sound": "返答の声",
	"How Muse's answers sound":                   "Muse の返答の声",
	"Home Assistant: Settings, Voice assistants": "Home Assistant: 設定 → 音声アシスタント",
	"Listening": "聞き取り中",
	"Quiet":     "静か", "Quiet hours": "おやすみ時間", "Do not disturb": "おやすみモード",
	"Alarms and timers will make no sound": "アラームとタイマーは鳴りません",
	"Alarms and timers, not the music":     "アラームとタイマー。音楽は含みません",
	"Sleep timer":                          "スリープタイマー", "Music": "音楽", "Audio output": "音声の出力先",
	"Where the sound goes with headphones in": "ヘッドホン接続時の音の出力先",
	"Bluetooth audio":                         "Bluetooth オーディオ", "Pair a new device": "新しい機器とペアリング",
	"Put it in pairing mode first": "先に相手をペアリングモードにしてください",
	"AirPlay":                      "AirPlay", "Play to it from an iPhone, iPad or Mac": "iPhone・iPad・Mac から再生",
	"Spotify Connect": "Spotify Connect", "Play to it from the Spotify app (Premium)": "Spotify アプリから再生（Premium）",
	"DLNA": "DLNA", "Play to it from music apps and servers": "音楽アプリやサーバーから再生",
	"Music Assistant player": "Music Assistant プレーヤー", "Play music in sync with other rooms": "他の部屋と同時に音楽を再生",
	"Home Assistant sounds":                            "Home Assistant の音",
	"Another speaker's music, while this one is quiet": "この端末が静かな間、他のスピーカーの音楽",
	"Camera sound":                                     "カメラの音", "A camera's own audio, while its view is up": "カメラ映像を表示中、そのカメラの音を鳴らす",
	"Camera time": "カメラの表示時間", "How long a camera opened here stays up": "ここで開いたカメラを表示しておく時間",
	"Cameras": "カメラ",

	// Alarms.
	"Alarm sound": "アラーム音", "Ring volume": "鳴る音量", "Snooze length": "スヌーズの長さ",
	"Rings from this device itself": "この端末自身で鳴らす", "Plays once when you choose it": "選ぶと 1 回鳴らします",
	"No alarms yet": "アラームはまだありません", "Add one here, or from Home Assistant": "ここか Home Assistant から追加",
	"Days": "曜日", "Hour": "時", "Minute": "分", "Repeat": "繰り返し", "Delete alarm": "アラームを削除",
	"New timer": "新しいタイマー", "Wake with light": "光で起こす", "Sun with a face": "顔のある太陽",
	"A face on it, for whoever has to look at it": "見る人のために顔を付ける",
	"Calendars": "カレンダー", "Event pop-ups": "予定のポップアップ", "Pop-ups": "ポップアップ",
	"Pop-up calendars": "ポップアップするカレンダー", "Pop-up chime": "ポップアップの音",
	"An event coming up, over the screen":             "近づいた予定を画面に重ねて表示",
	"Shown on the calendar page, from Home Assistant": "Home Assistant から、カレンダーページに表示",
	"Silent at night and in quiet hours":              "夜間とおやすみ時間は鳴らさない", "All-day events": "終日の予定",

	// Connections.
	"Wi-Fi": "Wi-Fi", "Bluetooth proxy": "Bluetooth プロキシ",
	"Lets Home Assistant hear nearby devices": "Home Assistant が近くの機器を検出できるようにする",
	"Call button": "通話ボタン", "On the home screen: devices and contacts": "ホーム画面に: 端末と連絡先",
	"Call ring": "着信音", "How a call rings here": "この端末での着信の鳴り方",
	"Allow Drop In": "Drop In を許可", "Intercom calls connect by themselves, after a chime": "チャイムのあと、インターホンが自動でつながる",
	"Intercom calls from other rooms are turned away": "他の部屋からのインターホンは断る",

	// Privacy & Security.
	"SSH": "SSH", "SSH keys": "SSH の鍵", "Sent from Home Assistant": "Home Assistant から送信",
	"Port 22, keys only": "ポート 22、鍵認証のみ",
	"Settings lock":      "設定のロック", "A PIN before these settings open": "設定を開く前に PIN を求める",
	"Camera on the network": "カメラをネットワークに公開", "Screen on the network": "画面をネットワークに公開",
	"Talk through cameras": "カメラ越しに話す", "Talk on the camera page": "カメラのページで話す",
	"Setup page": "セットアップページ", "A browser is asking: press the action button": "ブラウザが接続を求めています",
	"Home Assistant link": "Home Assistant との接続", "Encrypted with this device's key": "この端末の鍵で暗号化",
	"Certificate checks": "証明書の確認", "For downloads; updates always check": "ダウンロード時。アップデートは常に確認",

	// General.
	"Name": "名前", "Time zone": "タイムゾーン", "Updates": "アップデート", "About": "この端末について",
	"Its dates, its weather, and what it listens for": "日付・天気・聞き取る言葉",
	"Change it on the setup page":                     "セットアップページで変更", "For muting and timers": "ミュートとタイマー用",
	"Shown with the clock": "時計と一緒に表示",
}

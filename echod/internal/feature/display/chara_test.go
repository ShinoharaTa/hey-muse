//go:build !dot && !spot

package display

import (
	"image"
	"image/color"
	"image/draw"
	"image/png"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/HuskerMinion/techo5/echod/internal/feature/home"
	"github.com/HuskerMinion/techo5/echod/internal/feature/timer"
	"github.com/HuskerMinion/techo5/echod/internal/lib/hass"
)

// testChara is a character made of plain shapes: a pale blob with a dark band for eyes, on a
// transparent background, with a darker room behind it. Enough to see where everything lands.
func testChara() *charaSet {
	blob := func(busy bool) image.Image {
		im := image.NewRGBA(image.Rect(0, 0, 320, 440))
		pale, dark := color.RGBA{240, 246, 250, 255}, color.RGBA{50, 56, 70, 255}
		for y := 40; y < 420; y++ {
			for x := 30; x < 290; x++ {
				im.SetRGBA(x, y, pale)
			}
		}
		draw.Draw(im, image.Rect(110, 120, 210, 150), image.NewUniform(dark), image.Point{}, draw.Src)
		if busy {
			draw.Draw(im, image.Rect(90, 330, 230, 400), image.NewUniform(color.RGBA{90, 100, 120, 255}), image.Point{}, draw.Src)
		}
		return im
	}
	bg := image.NewRGBA(image.Rect(0, 0, 960, 480))
	for y := range 480 {
		draw.Draw(bg, image.Rect(0, y, 960, y+1), image.NewUniform(lerp(color.RGBA{26, 32, 48, 255}, color.RGBA{58, 44, 38, 255}, float64(y)/479)), image.Point{}, draw.Src)
	}
	return &charaSet{
		charaConfig: charaConfig{Name: "Sarah", Place: "東京", Lines: []string{"ねえ、何か用事ある？"}},
		bg:          bg, idle: blob(false), busy: blob(true),
		cache: map[image.Point]*charaScaled{},
	}
}

// pinCharaView makes the saved view v for one test, so the device's own view.json does not change what
// a test draws.
func pinCharaView(t *testing.T, v charaView) {
	charaViewGet()
	charaViewMu.Lock()
	old := charaViewNow
	charaViewNow = v
	charaViewMu.Unlock()
	t.Cleanup(func() {
		charaViewMu.Lock()
		charaViewNow = old
		charaViewMu.Unlock()
	})
}

// TestCharaScenesDraw draws the character's pages, and writes them as PNGs when SHOW_PREVIEW names a
// directory, the way TestShowScenesDraw does.
func TestCharaScenesDraw(t *testing.T) {
	pinCharaView(t, charaView{})
	at := time.Date(2026, 11, 7, 14, 50, 0, 0, time.Local)
	c := testChara()
	week := []hass.Day{
		{When: at, Condition: "sunny", High: 22, Low: 12, Rain: 0},
		{When: at.AddDate(0, 0, 1), Condition: "partlycloudy", High: 18, Low: 11, Rain: 10},
	}
	sky := home.Weather{Condition: "sunny", Temp: "20°"}
	long := "そうだね、午後はしっかり降るから、午前中だけならまだ大丈夫だし、急ぎじゃなければ金曜が晴れの予報だよ。" +
		"洗濯物は早めに取り込んでおくといいかも。傘は折りたたみで十分だと思う。"
	scenes := map[string]scene{
		"chara-home":            {now: at, phase: "idle", chara: c, weather: sky, forecast: week},
		"chara-home-no-weather": {now: at, phase: "idle", chara: c},
		"chara-home-timer":      {now: at, phase: "idle", chara: c, weather: sky, forecast: week, timers: []timer.Countdown{{Name: "Pasta", Left: 3*time.Minute + 20*time.Second}}, muted: true},
		"chara-listening":       {now: at, phase: "listening", chara: c},
		"chara-thinking":        {now: at, phase: "thinking", chara: c, heard: "明日の天気は？"},
		"chara-reply":           {now: at, phase: "replying", chara: c, heard: "明日の天気は？", reply: "明日は晴れのち曇り。最高18度、最低11度だよ。", voiceAudio: true},
		"chara-reply-long":      {now: at, phase: "lingering", chara: c, heard: "午後から雨って本当？", reply: long},
	}
	dir := os.Getenv("SHOW_PREVIEW")
	for name, s := range scenes {
		img := image.NewRGBA(image.Rect(0, 0, showWide, showHigh))
		r := newRenderer(img)
		r.draw(s)
		if s.phase == "idle" && len(r.homeZones) < 5 {
			t.Errorf("%s: %d tap zones, want the weather and four buttons", name, len(r.homeZones))
		}
		if dir == "" {
			continue
		}
		f, err := os.Create(filepath.Join(dir, name+".png"))
		if err != nil {
			t.Fatal(err)
		}
		if err := png.Encode(f, img); err != nil {
			t.Fatal(err)
		}
		if err := f.Close(); err != nil {
			t.Fatal(err)
		}
	}
}

// TestCharaLoadsFromDir reads a set the way the device does, and refuses a directory with no idle.png.
func TestCharaLoadsFromDir(t *testing.T) {
	dir := t.TempDir()
	if _, err := loadCharaFrom(dir); err == nil {
		t.Fatal("a directory without idle.png loaded")
	}
	c := testChara()
	for name, img := range map[string]image.Image{"idle.png": c.idle, "background.png": c.bg} {
		f, err := os.Create(filepath.Join(dir, name))
		if err != nil {
			t.Fatal(err)
		}
		if err := png.Encode(f, img); err != nil {
			t.Fatal(err)
		}
		f.Close()
	}
	if err := os.WriteFile(filepath.Join(dir, "chara.json"), []byte(`{"name":"Sarah","lines":["hi"]}`), 0o644); err != nil {
		t.Fatal(err)
	}
	set, err := loadCharaFrom(dir)
	if err != nil {
		t.Fatal(err)
	}
	if set.Name != "Sarah" || set.bg == nil || set.busy != nil {
		t.Fatalf("loaded %q bg=%v busy=%v", set.Name, set.bg != nil, set.busy != nil)
	}
	sc := set.scaled(showWide, showHigh)
	if sc.bg.Bounds().Dx() != showWide || sc.idle[0].Bounds().Dy() != showHigh*86/100 || sc.busy == nil {
		t.Fatalf("scaled bg=%v idle=%v", sc.bg.Bounds(), sc.idle[0].Bounds())
	}
}

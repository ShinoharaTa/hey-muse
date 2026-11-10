//go:build !dot && !spot

package display

import (
	"image"
	"image/png"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/HuskerMinion/techo5/echod/internal/feature/home"
	"github.com/HuskerMinion/techo5/echod/internal/lib/hass"
)

// TestRoomHourFollowsTheSun puts each hour in its part of a November day in Tokyo, and uses fixed
// hours when the sun's times are not known.
func TestRoomHourFollowsTheSun(t *testing.T) {
	at := func(h, m int) time.Time { return time.Date(2026, 11, 10, h, m, 0, 0, time.Local) }
	rise, set := at(6, 13), at(16, 35)
	for _, c := range []struct {
		h, m int
		want string
	}{
		{3, 0, roomNight}, {5, 12, roomNight}, {5, 13, roomDawn}, {6, 42, roomDawn}, {6, 43, roomMorning},
		{9, 59, roomMorning}, {10, 0, roomDay}, {15, 34, roomDay}, {15, 35, roomEvening}, {17, 14, roomEvening},
		{17, 15, roomNight}, {23, 0, roomNight},
	} {
		if got := roomHour(at(c.h, c.m), rise, set, true); got != c.want {
			t.Errorf("%02d:%02d = %s, want %s", c.h, c.m, got, c.want)
		}
	}
	if got := roomHour(at(5, 30), time.Time{}, time.Time{}, false); got != roomDawn {
		t.Errorf("5:30 without the sun's times = %s, want dawn", got)
	}
	if got := roomHour(at(17, 0), time.Time{}, time.Time{}, false); got != roomEvening {
		t.Errorf("17:00 without the sun's times = %s, want evening", got)
	}
	// A summer sunset after six: the afternoon stays day until five.
	if got := roomHour(at(16, 59), rise, at(18, 0), true); got != roomDay {
		t.Errorf("16:59 with sunset at 18:00 = %s, want day", got)
	}
}

func TestRoomSky(t *testing.T) {
	for cond, want := range map[string]string{
		"sunny": roomClear, "clear-night": roomClear, "partlycloudy": roomClear, "": roomClear,
		"cloudy": roomCloudy, "fog": roomCloudy, "snowy": roomCloudy,
		"rainy": roomRain, "pouring": roomRain, "lightning-rainy": roomRain,
	} {
		if got := roomSky(cond); got != want {
			t.Errorf("%q = %s, want %s", cond, got, want)
		}
	}
}

// TestRoomFallsBack uses the hour's clear room for a missing weather, and background.png for a missing hour.
func TestRoomFallsBack(t *testing.T) {
	c := &charaSet{rooms: map[string]bool{"night_clear.png": true, "night_rain.png": true}}
	for _, k := range []struct{ hour, sky, want string }{
		{roomNight, roomRain, "night_rain.png"}, {roomNight, roomCloudy, "night_clear.png"}, {roomDay, roomRain, ""},
	} {
		if got := c.roomFile(k.hour, k.sky); got != k.want {
			t.Errorf("%s %s = %q, want %q", k.hour, k.sky, got, k.want)
		}
	}
}

// TestRoomLoadsAndSwaps reads a room off the frame: the first ask has none, a later one has it, and
// asking for another hour keeps the old room on screen until the new one is ready.
func TestRoomLoadsAndSwaps(t *testing.T) {
	dir := t.TempDir()
	os.Mkdir(filepath.Join(dir, charaRoomsDir), 0o755)
	for _, name := range []string{"day_clear.png", "night_clear.png"} {
		f, _ := os.Create(filepath.Join(dir, charaRoomsDir, name))
		png.Encode(f, image.NewRGBA(image.Rect(0, 0, 96, 48)))
		f.Close()
	}
	os.WriteFile(filepath.Join(dir, "not-a-room.png"), nil, 0o644)
	c := &charaSet{dir: dir, rooms: findRooms(dir)}
	if len(c.rooms) != 2 {
		t.Fatalf("found %v", c.rooms)
	}
	defer func(f func(time.Time) (time.Time, time.Time, bool)) { charaRoomSun = f }(charaRoomSun)
	charaRoomSun = func(time.Time) (time.Time, time.Time, bool) { return time.Time{}, time.Time{}, false }
	noon := time.Date(2026, 11, 10, 12, 0, 0, 0, time.Local)
	wait := func(at time.Time, want string) *image.RGBA {
		for range 200 {
			if img := c.roomAt(60, 30, at, "sunny"); img != nil && c.roomShown() == want {
				return img
			}
			time.Sleep(10 * time.Millisecond)
		}
		t.Fatalf("%s never shown; showing %q", want, c.roomShown())
		return nil
	}
	if img := wait(noon, "day_clear.png"); img.Bounds().Dx() != 60 {
		t.Fatalf("scaled to %v", img.Bounds())
	}
	if c.roomAt(60, 30, noon.Add(10*time.Hour), "sunny") == nil {
		t.Fatal("the day room left the screen before the night one was ready")
	}
	wait(noon.Add(10*time.Hour), "night_clear.png")
}

// TestCharaRoomsPreview draws the idle page in every room of a real character set: CHARA_DIR names
// the set (the device's /data/misc/techo5/chara) and SHOW_PREVIEW where the PNGs go.
func TestCharaRoomsPreview(t *testing.T) {
	src, out := os.Getenv("CHARA_DIR"), os.Getenv("SHOW_PREVIEW")
	if src == "" || out == "" {
		t.Skip("CHARA_DIR and SHOW_PREVIEW not set")
	}
	pinCharaView(t, charaView{})
	c, err := loadCharaFrom(src)
	if err != nil {
		t.Fatal(err)
	}
	defer func(f func(time.Time) (time.Time, time.Time, bool)) { charaRoomSun = f }(charaRoomSun)
	day := time.Date(2026, 11, 10, 0, 0, 0, 0, time.Local)
	charaRoomSun = func(time.Time) (time.Time, time.Time, bool) {
		return day.Add(6*time.Hour + 13*time.Minute), day.Add(16*time.Hour + 35*time.Minute), true
	}
	hours := map[string]time.Duration{roomDawn: 5*time.Hour + 50*time.Minute, roomMorning: 8 * time.Hour,
		roomDay: 13 * time.Hour, roomEvening: 16*time.Hour + 30*time.Minute, roomNight: 21 * time.Hour}
	skies := map[string]string{roomClear: "sunny", roomCloudy: "cloudy", roomRain: "rainy"}
	for hour, off := range hours {
		for sky, cond := range skies {
			at := day.Add(off)
			if hour == roomNight && cond == "sunny" {
				cond = "clear-night"
			}
			want := c.roomFile(hour, sky)
			for range 500 {
				if c.roomAt(showWide, showHigh, at, cond); c.roomShown() == want {
					break
				}
				time.Sleep(10 * time.Millisecond)
			}
			img := image.NewRGBA(image.Rect(0, 0, showWide, showHigh))
			r := newRenderer(img)
			r.draw(scene{now: at, phase: "idle", chara: c, weather: home.Weather{Condition: cond, Temp: "14°"},
				forecast: []hass.Day{{When: at, Condition: cond, High: 18, Low: 9}, {When: at.AddDate(0, 0, 1), Condition: "sunny", High: 19, Low: 10}}})
			f, err := os.Create(filepath.Join(out, hour+"_"+sky+".png"))
			if err != nil {
				t.Fatal(err)
			}
			png.Encode(f, img)
			f.Close()
		}
	}
}

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
	"github.com/HuskerMinion/techo5/echod/internal/lib/openmeteo"
)

func TestComingUpSkipsWhatIsOverAndRepeats(t *testing.T) {
	now := time.Date(2026, 11, 10, 15, 30, 0, 0, time.Local)
	at := func(d, h, m int) time.Time { return time.Date(2026, 11, d, h, m, 0, 0, time.Local) }
	over := hass.Event{Summary: "朝会", Start: at(10, 9, 0), End: at(10, 9, 30)}
	running := hass.Event{Summary: "作業", Start: at(10, 15, 0), End: at(10, 17, 0)}
	allDay := hass.Event{Summary: "誕生日", Start: at(10, 0, 0), End: at(11, 0, 0), AllDay: true}
	later := hass.Event{Summary: "歯医者", Start: at(10, 18, 0), End: at(10, 19, 0)}
	tomorrow := hass.Event{Summary: "打ち合わせ", Start: at(11, 10, 0), End: at(11, 11, 0)}
	// An event across midnight comes back from both days.
	got := comingUp([]hass.Event{later, over, running, allDay, tomorrow, running}, now)
	if len(got) != agendaEvents || got[0].Summary != "誕生日" || got[1].Summary != "作業" {
		t.Fatalf("got %v", got)
	}
	for e, want := range map[*hass.Event]string{&running: "〜17:00", &allDay: "終日", &later: "18:00", &tomorrow: "明日 10:00"} {
		if got := agendaWhen(*e, now); got != want {
			t.Errorf("%s: %q, want %q", e.Summary, got, want)
		}
	}
}

func TestAgendaHoursAreEveryThreeHoursAhead(t *testing.T) {
	now := time.Date(2026, 11, 10, 15, 30, 0, 0, time.Local)
	var hours []openmeteo.Hour
	for h := range 24 {
		hours = append(hours, openmeteo.Hour{When: now.Truncate(time.Hour).Add(time.Duration(h) * time.Hour)})
	}
	got := agendaHours(hours, now)
	if len(got) != agendaSlots {
		t.Fatalf("%d hours", len(got))
	}
	for i, want := range []int{18, 21, 0, 3} {
		if got[i].When.Hour() != want {
			t.Errorf("slot %d at %d, want %d", i, got[i].When.Hour(), want)
		}
	}
}

// TestClockAgendaDraws draws the clock layout with the weather to come and the calendar, its empty
// states, and with only days of weather; SHOW_PREVIEW names a directory for the PNGs.
func TestClockAgendaDraws(t *testing.T) {
	pinCharaView(t, charaView{})
	now := time.Date(2026, 11, 10, 15, 30, 0, 0, time.Local)
	c := testChara()
	var hours []openmeteo.Hour
	for h, cond := range []string{"cloudy", "cloudy", "rainy", "rainy", "cloudy", "clear-night", "clear-night", "clear-night", "clear-night", "clear-night", "clear-night", "clear-night", "sunny", "sunny"} {
		hours = append(hours, openmeteo.Hour{When: now.Truncate(time.Hour).Add(time.Duration(h) * time.Hour), Condition: cond, Temp: 12 - float64(h)/3, Rain: 60 - 4*h})
	}
	days := []hass.Day{{When: now, Condition: "rainy", High: 13, Low: 7, Rain: 90}, {When: now.AddDate(0, 0, 1), Condition: "sunny", High: 17, Low: 6, Rain: 0}}
	events := charaAgenda{calendar: true, loaded: true, events: []hass.Event{
		{Summary: "歯医者", Start: now.Add(30 * time.Minute), End: now.Add(90 * time.Minute)},
		{Summary: "チームの打ち合わせ（オンライン）", Start: now.Add(19 * time.Hour), End: now.Add(20 * time.Hour)},
	}}
	sky := home.Weather{Condition: "cloudy", Temp: "12°"}
	scenes := map[string]scene{
		"agenda":            {now: now, phase: "idle", chara: c, weather: sky, weatherPlace: "さいたま, 埼玉県", forecast: days, hours: hours, agenda: events},
		"agenda-no-cal":     {now: now, phase: "idle", chara: c, weather: sky, weatherPlace: "さいたま, 埼玉県", forecast: days, hours: hours},
		"agenda-empty-days": {now: now, phase: "idle", chara: c, weather: sky, forecast: days, agenda: charaAgenda{calendar: true, loaded: true}},
	}
	dir := os.Getenv("SHOW_PREVIEW")
	for name, s := range scenes {
		img := image.NewRGBA(image.Rect(0, 0, showWide, showHigh))
		r := newRenderer(img)
		r.draw(s)
		var acts []string
		for _, z := range r.homeZones {
			acts = append(acts, z.action)
		}
		if len(acts) != 3 || acts[1] != "weather" || acts[2] != "calendar" {
			t.Errorf("%s: zones %v, want the character, the weather and the calendar", name, acts)
		}
		if dir != "" {
			f, _ := os.Create(filepath.Join(dir, name+".png"))
			png.Encode(f, img)
			f.Close()
		}
	}
}

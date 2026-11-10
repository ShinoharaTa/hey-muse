//go:build !dot && !spot

package display

import (
	"fmt"
	"image"
	"image/color"
	"slices"
	"time"

	"github.com/HuskerMinion/techo5/echod/internal/feature/home"
	"github.com/HuskerMinion/techo5/echod/internal/lib/hass"
	"github.com/HuskerMinion/techo5/echod/internal/lib/locale"
	"github.com/HuskerMinion/techo5/echod/internal/lib/openmeteo"
)

// "このあと" on the clock layout: the weather of the next hours and what is on the calendar for the rest
// of today and tomorrow, under the clock.

const (
	// agendaSlots is how many hours of the weather are shown, agendaStep hours apart.
	agendaSlots = 4
	agendaStep  = 3
	// agendaEvents is how many events are listed.
	agendaEvents = 2
)

// charaAgenda is what the calendar has coming: calendar false with no calendar chosen for the device,
// loaded false while it is still being read.
type charaAgenda struct {
	calendar, loaded bool
	events           []hass.Event
}

// agendaNow reads the calendar for the rest of today and tomorrow, from what is kept (it never waits).
func agendaNow(now time.Time) charaAgenda {
	h := home.Get()
	if len(h.CalendarSources()) == 0 {
		return charaAgenda{}
	}
	today, ok1 := h.EventsOn(now)
	tomorrow, ok2 := h.EventsOn(now.AddDate(0, 0, 1))
	return charaAgenda{calendar: true, loaded: ok1 && ok2, events: comingUp(append(today, tomorrow...), now)}
}

// comingUp is the events not over by now, each once (one running past midnight is on both days),
// soonest first, at most agendaEvents.
func comingUp(events []hass.Event, now time.Time) []hass.Event {
	var out []hass.Event
	for _, e := range events {
		if !e.End.After(now) && !(e.AllDay && sameDay(e.Start, now)) {
			continue
		}
		if slices.ContainsFunc(out, func(o hass.Event) bool {
			return o.Summary == e.Summary && o.Start.Equal(e.Start) && o.Calendar == e.Calendar
		}) {
			continue
		}
		out = append(out, e)
	}
	slices.SortStableFunc(out, func(a, b hass.Event) int { return a.Start.Compare(b.Start) })
	return out[:min(len(out), agendaEvents)]
}

// agendaWhen is how an event's time is put: "16:00", "〜17:00" for one under way, "終日", with "明日 "
// before tomorrow's.
func agendaWhen(e hass.Event, now time.Time) string {
	day := ""
	if !sameDay(e.Start, now) && e.Start.After(now) {
		day = "明日 "
	}
	switch {
	case e.AllDay:
		return day + "終日"
	case !e.Start.After(now):
		return "〜" + e.End.Format("15:04")
	}
	return day + e.Start.Format("15:04")
}

// agendaHours is the hours to show: every agendaStep hours on the hour after now, agendaSlots of them.
func agendaHours(hours []openmeteo.Hour, now time.Time) []openmeteo.Hour {
	var out []openmeteo.Hour
	for _, h := range hours {
		if h.When.After(now) && h.When.Hour()%agendaStep == 0 {
			out = append(out, h)
			if len(out) == agendaSlots {
				break
			}
		}
	}
	return out
}

// drawClockAgenda is the clock layout: the clock and the date, the weather now and the next hours',
// the calendar, and the status pill. Everything stays left of x=470, clear of the
// character's poses at the desk and on the cushion.
func (r *renderer) drawClockAgenda(s scene, date string, rect func(x, y, w, h int) image.Rectangle,
	text func(size int, bold bool, v string, x, y int, c color.RGBA)) []homeZone {
	ink, muted := charaInk.text, charaInk.muted
	text(150, true, s.now.Format("15:04"), 30, 140, ink)
	text(26, false, date, 38, 180, muted)

	var zones []homeZone
	w := rect(36, 196, 430, 104)
	r.glassTile(w, 16)
	text(16, false, "このあとの天気", 54, 218, muted)
	zones = append(zones, homeZone{w, "weather"})
	// The weather now, at the right of the heading: the place, the sky and the temperature.
	if s.weather.Condition != "" {
		tf := r.styleFace(true, 18)
		r.rightText(tf, s.weather.Temp, r.s(448), r.s(219), ink)
		x := 448 - r.unscale(r.width(tf, s.weather.Temp)) - 16
		r.weatherIcon(s.weather.Condition, r.s(x), r.s(213), r.s(22))
		now := "いま"
		if place := charaPlace(s.chara, s.weatherPlace); place != "" {
			now = place + " · いま"
		}
		r.rightText(r.styleFace(false, 15), now, r.s(x-16), r.s(218), muted)
	}
	if hours := agendaHours(s.hours, s.now); len(hours) > 0 {
		for i, h := range hours {
			x := 54 + i*103
			text(17, false, fmt.Sprintf("%d時", h.When.Hour()), x, 244, muted)
			r.weatherIcon(h.Condition, r.s(x+16), r.s(270), r.s(30))
			text(21, true, fmt.Sprintf("%.0f°", h.Temp+0), x+38, 270, ink)
			if h.Rain >= 0 {
				text(15, false, fmt.Sprintf("雨%d%%", h.Rain), x+38, 290, muted)
			}
		}
	} else if today, ok := charaDay(s.forecast, s.now); ok {
		// No hours (Home Assistant's weather gives days): today and tomorrow instead.
		tomorrow, okT := charaDay(s.forecast, s.now.AddDate(0, 0, 1))
		for i, d := range []struct {
			head string
			day  hass.Day
			ok   bool
		}{{"今日", today, true}, {"明日", tomorrow, okT}} {
			if !d.ok {
				continue
			}
			x := 54 + i*206
			text(17, false, d.head, x, 244, muted)
			r.weatherIcon(d.day.Condition, r.s(x+18), r.s(272), r.s(32))
			text(20, true, fmt.Sprintf("%.0f° / %.0f°", d.day.High, d.day.Low), x+44, 270, ink)
			text(15, false, locale.Sky(d.day.Condition, "ja"), x+44, 290, muted)
		}
	} else {
		line := "天気を取得中"
		if !s.forecastAt.IsZero() {
			line = "天気を更新できません"
		}
		text(20, false, line, 54, 266, ink)
	}

	a := rect(36, 308, 430, 92)
	r.glassTile(a, 16)
	text(16, false, "このあとの予定", 54, 330, muted)
	zones = append(zones, homeZone{a, "calendar"})
	switch {
	case !s.agenda.calendar:
		text(18, false, "カレンダーが未登録です", 54, 362, ink)
		text(15, false, "セットアップページで追加できます", 54, 386, muted)
	case len(s.agenda.events) == 0 && !s.agenda.loaded:
		text(18, false, "予定を読み込み中", 54, 362, ink)
	case len(s.agenda.events) == 0:
		text(18, false, "今日と明日の予定はありません", 54, 362, ink)
	default:
		for i, e := range s.agenda.events {
			y := 360 + i*28
			when := agendaWhen(e, s.now)
			text(18, true, when, 54, y, charaInk.accent)
			f := r.styleFace(false, 19)
			x := 54 + r.unscale(r.width(r.styleFace(true, 18), when)) + 12
			r.shadowText(f, r.fit(f, e.Summary, r.s(450-x)), r.s(x), r.s(y), ink)
		}
	}
	r.charaPill(s, 36, 412, 430)
	return zones
}

// unscale is a width measured on this screen in the Show 5's pixels, which the layout is written in.
func (r *renderer) unscale(px int) int {
	if r.sDen == 0 || r.sNum == r.sDen {
		return px
	}
	return (px*r.sDen + r.sNum/2) / r.sNum
}

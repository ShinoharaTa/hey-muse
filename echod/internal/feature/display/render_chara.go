//go:build !dot && !spot

package display

import (
	"fmt"
	"image"
	"image/color"
	"image/draw"
	"time"

	"golang.org/x/image/font"

	"github.com/HuskerMinion/techo5/echod/internal/lib/hass"
	"github.com/HuskerMinion/techo5/echod/internal/lib/locale"
)

// The resident character's pages (chara.go, anim_chara.go). The idle page is the room: the character
// standing at the right, the clock, the weather in two glass tiles, a row of buttons and a status
// pill at the left, and now and then a line in a speech bubble. A turn is a chat with the character:
// what was heard in a bubble on the right, the answer in one on the left, and a card saying the
// character is at work while the answer is on its way. Both are laid out in the Show 5's pixels,
// scaled by r.s.
//
// The idle page's furniture (room, clock, tiles, buttons, pill) changes once a minute at most, so it
// is drawn into a picture kept between frames, and a frame of the character moving costs a copy of
// that, the character and the bubble.

// charaInk is the idle page's palette: light words over the room, which is dark or washed.
var charaInk = struct{ text, muted, glass, rim, accent, ready, bubble, bubbleText, spark color.RGBA }{
	color.RGBA{248, 248, 244, 255}, color.RGBA{205, 212, 218, 255},
	color.RGBA{16, 22, 30, 255}, color.RGBA{255, 255, 255, 255},
	color.RGBA{139, 217, 192, 255}, color.RGBA{86, 214, 128, 255},
	color.RGBA{255, 255, 255, 255}, color.RGBA{40, 46, 54, 255},
	color.RGBA{255, 170, 196, 255},
}

// charaChat is the turn page's palette: a pale room, the way a chat app looks.
var charaChat = struct{ top, bottom, pill, text, muted, mine, mineText, theirs, accent, line color.RGBA }{
	color.RGBA{222, 234, 241, 255}, color.RGBA{243, 247, 250, 255},
	color.RGBA{206, 222, 231, 255}, color.RGBA{38, 48, 56, 255}, color.RGBA{110, 126, 138, 255},
	color.RGBA{34, 128, 126, 255}, color.RGBA{250, 253, 252, 255},
	color.RGBA{255, 255, 255, 255}, color.RGBA{34, 128, 126, 255}, color.RGBA{190, 206, 216, 255},
}

// charaLineEvery is how long each speech-bubble line stays before the next.
const charaLineEvery = 3 * time.Minute

// breathFrame is how often a breathing frame is drawn by day.
const breathFrame = 166 * time.Millisecond

// charaLine is the bubble's line now, or empty with none configured.
func (c *charaSet) charaLine(now time.Time) string {
	if len(c.Lines) == 0 {
		return ""
	}
	return c.Lines[int(now.Unix()/int64(charaLineEvery/time.Second))%len(c.Lines)]
}

// pettedLine is the line for a pat on the head, the ordinary one with none configured.
func (c *charaSet) pettedLine(at time.Time) string {
	if len(c.Petted) == 0 {
		return c.charaLine(at)
	}
	return c.Petted[int(at.UnixMilli()/97)%len(c.Petted)]
}

// charaOn is whether the idle page is the character's: a set is installed and the device is idle,
// with no page of its own (the sunrise, the screensaver, the music) in the way. The drawer, the
// music strip, the volume bar and the cards come over it, as they do over the clock.
func charaOn(s scene) bool {
	return s.chara != nil && s.phase == "idle" && s.sunrise == 0 && s.slideshowScreensaver == nil && !s.nowPlaying
}

// charaTurn is whether a turn is drawn as the chat: a set is installed and a turn is under way.
func charaTurn(s scene) bool {
	if s.chara == nil {
		return false
	}
	switch s.phase {
	case "listening", "thinking", "replying", "lingering":
		return true
	}
	return false
}

// wantFrame asks for the next frame no later than d from now; the soonest ask wins.
func (r *renderer) wantFrame(d time.Duration) {
	if d <= 0 {
		return
	}
	if r.charaNext == 0 || d < r.charaNext {
		r.charaNext = d
	}
}

// shadowText draws words with a soft dark edge under them, so they read over any room.
func (r *renderer) shadowText(f font.Face, v string, x, y int, c color.RGBA) {
	r.text(f, v, x+r.s(2), y+r.s(2), color.RGBA{0, 0, 0, 110})
	r.text(f, v, x, y, c)
}

// glassTile is one see-through tile: dark glass with a light rim.
func (r *renderer) glassTile(b image.Rectangle, rad int) {
	r.glassFill(b, float64(r.s(rad)), charaInk.glass, 0.42, 0.56)
	r.glassRim(b, float64(r.s(rad)), float64(r.s(3))/2, charaInk.rim, 0.42)
}

// charaDay finds the forecast for a calendar day, when there is one.
func charaDay(days forecastDays, day time.Time) (hass.Day, bool) {
	want := day.Local().Format("2006-01-02")
	for _, d := range days {
		if d.When.Local().Format("2006-01-02") == want {
			return d, true
		}
	}
	return hass.Day{}, false
}

// charaFloorAt is where the standing picture's left edge and floor are on this screen.
func (r *renderer) charaFloorAt(sc *charaScaled) (x, floor int) {
	return r.w - sc.idle[0].Bounds().Dx() - r.s(24), r.h
}

// charaFurnitureKey is what the idle page's furniture depends on.
func charaFurnitureKey(s scene) string {
	timer := ""
	if len(s.timers) > 0 {
		timer = s.timers[0].Left.Round(time.Second).String()
	}
	return fmt.Sprintf("%s|%s|%v|%s|%s|%s|%d|%v", s.now.Format("2006-01-02 15:04"), s.weatherPlace, s.muted, timer,
		s.weather.Condition, s.weather.Temp, len(s.forecast), s.forecastAt.IsZero())
}

// drawCharaHome is the idle page.
func (r *renderer) drawCharaHome(s scene) {
	c := s.chara
	sc := c.scaled(r.w, r.h)
	now := s.now
	night := nightNow(now)

	key := charaFurnitureKey(s)
	if r.charaStatic == nil || r.charaStatic.Bounds() != r.dst.Bounds() || r.charaStaticKey != key {
		r.charaFurniture(s, sc)
		r.charaStaticKey = key
	}
	draw.Draw(r.dst, r.dst.Rect, r.charaStatic, image.Point{}, draw.Src)

	// The character: its breathing frame by day, its hop, its eyes.
	a := &r.chara
	a.phase = "idle"
	x, floor := r.charaFloorAt(sc)
	dy, shape, bounceNext := a.bounce(now)
	r.wantFrame(bounceNext)
	eyes, blinkNext := a.blink(now, night, sc.idleExpr[exprEyesHalf].img != nil)
	r.wantFrame(blinkNext)
	if blinkNext == 0 {
		if until := a.untilBlink(now); until > 0 && until < 2*time.Second {
			r.wantFrame(until)
		}
	}
	smile, sparkle, petting := a.petted(now)
	frame := 0
	if !night && eyes == "" && shape == "" && !petting {
		frame = breath(now)
		r.wantFrame(breathFrame)
	}
	pic := sc.idle[frame]
	switch shape {
	case "squash":
		pic = sc.squash
	case "stretch":
		pic = sc.stretch
	}
	px := x + (sc.idle[0].Bounds().Dx()-pic.Bounds().Dx())/2
	at := image.Pt(px, floor-pic.Bounds().Dy()+dy)
	draw.Draw(r.dst, pic.Bounds().Add(at), pic, image.Point{}, draw.Over)
	if shape == "" {
		// The face sits where it does on the resting picture, lifted by what the breath added under it.
		expr := eyes
		if smile && sc.idleExpr[exprHappy].img != nil {
			expr = exprHappy
		}
		if p, ok := sc.idleExpr[expr]; ok && expr != "" {
			h0 := sc.idle[0].Bounds().Dy()
			grown := float64(pic.Bounds().Dy()-h0) / float64(h0)
			lift := int(float64(h0-p.at.Y)*grown + 0.5)
			drawPatch(r.dst, sc.idleExpr, expr, image.Pt(px, floor-h0+dy), -lift)
		}
	}
	if petting && sparkle < 1 {
		// Three sparks rising from the head and fading.
		head := image.Pt(x+sc.idle[0].Bounds().Dx()/2, floor-sc.idle[0].Bounds().Dy()+r.s(30))
		for i, off := range []image.Point{{-50, 0}, {0, -16}, {46, 4}} {
			p := head.Add(image.Pt(r.s(off.X), r.s(off.Y)-int(float64(r.s(20))*sparkle)))
			rad := r.s(7 - i)
			r.glassFill(image.Rect(p.X-rad, p.Y-rad, p.X+rad, p.Y+rad), float64(rad), charaInk.spark, 1-sparkle, 1-sparkle)
		}
		r.wantFrame(80 * time.Millisecond)
	}

	// The speech bubble, over the character's head, rising into place when its line changes.
	line := c.charaLine(now)
	if petting || (!a.petAt.IsZero() && now.Sub(a.petAt) < 4*time.Second) {
		line = c.pettedLine(a.petAt)
	}
	if line != "" {
		rise, moving := a.bubble(now, line)
		if moving {
			r.wantFrame(80 * time.Millisecond)
		}
		f := r.styleFace(false, 22)
		w := min(r.width(f, line)+r.s(44), r.s(400))
		line = r.fit(f, line, w-r.s(44))
		x1 := r.w - r.s(30)
		top := r.s(26) + int(float64(r.s(6))*(1-rise))
		bb := image.Rect(x1-w, top, x1, top+r.s(50))
		r.roundFill(bb, float64(r.s(25)), charaInk.bubble, charaInk.bubble)
		r.text(f, line, bb.Min.X+r.s(22), top+r.s(34), charaInk.bubbleText)
	}

	// Taps: the character itself, the weather, and the buttons.
	body := sc.idle[0].Bounds().Add(image.Pt(x, floor-sc.idle[0].Bounds().Dy()))
	zones := append([]homeZone{{body, "pet"}}, r.charaZones...)
	r.zmu.Lock()
	r.homeZones = zones
	r.zmu.Unlock()
}

// charaFurniture draws everything on the idle page but the character and the bubble into the
// picture kept between frames, and remembers where its buttons are.
func (r *renderer) charaFurniture(s scene, sc *charaScaled) {
	c := s.chara
	if r.charaStatic == nil || r.charaStatic.Bounds() != r.dst.Bounds() {
		r.charaStatic = image.NewRGBA(r.dst.Bounds())
	}
	// Draw into the kept picture by swapping it in as the canvas for the duration.
	dst := r.dst
	r.dst = r.charaStatic
	defer func() { r.dst = dst }()

	if sc.bg != nil {
		draw.Draw(r.dst, r.dst.Rect, sc.bg, image.Point{}, draw.Src)
	} else {
		r.glassBackdrop()
	}
	rect := func(x, y, w, h int) image.Rectangle { return image.Rect(r.s(x), r.s(y), r.s(x+w), r.s(y+h)) }
	text := func(size int, bold bool, v string, x, y int, c color.RGBA) {
		r.shadowText(r.styleFace(bold, size), v, r.s(x), r.s(y), c)
	}
	ink, muted := charaInk.text, charaInk.muted

	// The clock and the date.
	text(108, true, s.now.Format("15:04"), 30, 150, ink)
	text(24, false, fmt.Sprintf("%d年%d月%d日（%s）", s.now.Year(), s.now.Month(), s.now.Day(), []string{"日", "月", "火", "水", "木", "金", "土"}[s.now.Weekday()]), 36, 190, muted)

	// Two weather tiles: today and tomorrow. With no forecast yet, one tile says what is known.
	place := c.Place
	if place == "" {
		place = s.weatherPlace
	}
	label := func(day string) string {
		if place == "" {
			return day
		}
		return day + "・" + place
	}
	tile := func(x int, head string, d hass.Day, ok bool) {
		b := rect(x, 214, 274, 104)
		r.glassTile(b, 16)
		text(19, false, head, x+18, 240, muted)
		if !ok {
			line := "天気を取得中"
			if s.weather.Condition != "" {
				line = locale.Sky(s.weather.Condition, "ja") + "  " + s.weather.Temp
			} else if !s.forecastAt.IsZero() {
				line = "天気を更新できません"
			}
			text(22, false, line, x+18, 282, ink)
			return
		}
		r.weatherIcon(d.Condition, r.s(x+240), r.s(238), r.s(40))
		f := r.styleFace(true, 28)
		r.shadowText(f, r.fit(f, locale.Sky(d.Condition, "ja"), r.s(200)), r.s(x+18), r.s(275), ink)
		line := fmt.Sprintf("%.0f° / %.0f°", d.High, d.Low)
		if d.Rain >= 0 {
			line += fmt.Sprintf("  雨%d%%", d.Rain)
		}
		text(20, false, line, x+18, 306, ink)
	}
	today, okToday := charaDay(s.forecast, s.now)
	tomorrow, okTomorrow := charaDay(s.forecast, s.now.AddDate(0, 0, 1))
	tile(36, label("今日"), today, okToday)
	tile(326, label("明日"), tomorrow, okTomorrow)
	zones := []homeZone{{rect(36, 214, 564, 104), "weather"}}

	// The buttons.
	buttons := []struct{ label, action, icon string }{
		{"話す", "talk", "mdi:microphone"}, {"ラジオ", "radio", "mdi:radio"},
		{"天気", "weather", "mdi:weather-partly-cloudy"}, {"設定", "settings", "mdi:cog-outline"},
	}
	for i, bt := range buttons {
		x := 36 + i*145
		b := rect(x, 336, 131, 54)
		r.glassTile(b, 14)
		r.mdiIcon(bt.icon, r.s(x+14), r.s(349), 28, charaInk.accent)
		text(22, true, bt.label, x+50, 371, ink)
		zones = append(zones, homeZone{b, bt.action})
	}

	// The status pill: what the device is doing, and what to say to it.
	b := rect(36, 408, 564, 46)
	r.glassTile(b, 23)
	dot, status := charaInk.ready, "Ready · Hey Muse"
	if s.muted {
		dot, status = color.RGBA{232, 98, 98, 255}, "マイクオフ"
	}
	r.roundFill(rect(54, 424, 14, 14), float64(r.s(7)), dot, dot)
	text(20, false, status, 80, 439, ink)
	if len(s.timers) > 0 {
		left := s.timers[0].Left.Round(time.Second)
		r.rightText(r.styleFace(false, 20), fmt.Sprintf("タイマー %d:%02d", int(left.Minutes()), int(left.Seconds())%60), r.s(582), r.s(439), charaInk.accent)
	}
	r.charaZones = zones
}

// drawCharaTurn is the chat page a turn is shown as.
func (r *renderer) drawCharaTurn(s scene) {
	c := s.chara
	sc := c.scaled(r.w, r.h)
	p := charaChat
	now := s.now
	a := &r.chara
	a.turnPhase(now, s.phase)
	dy, _, bounceNext := a.bounce(now)
	r.wantFrame(bounceNext)
	// The room: pale, a little darker at the top.
	for y := 0; y < r.h; y++ {
		col := lerp(p.top, p.bottom, float64(y)/float64(max(r.h-1, 1)))
		draw.Draw(r.dst, image.Rect(0, y, r.w, y+1), image.NewUniform(col), image.Point{}, draw.Src)
	}
	rect := func(x, y, w, h int) image.Rectangle { return image.Rect(r.s(x), r.s(y), r.s(x+w), r.s(y+h)) }
	text := func(size int, bold bool, v string, x, y int, c color.RGBA) {
		r.text(r.styleFace(bold, size), v, r.s(x), r.s(y), c)
	}

	// The header: the way back, who this is and what they are doing, the time.
	r.roundFill(rect(24, 16, 150, 44), float64(r.s(22)), p.pill, p.pill)
	text(22, false, "‹ 待機画面", 44, 46, p.text)
	ib := sc.icon.Bounds()
	draw.Draw(r.dst, ib.Add(image.Pt(r.s(196), r.s(38)-ib.Dy()/2+dy)), sc.icon, ib.Min, draw.Over)
	state := "オンライン"
	switch s.phase {
	case "listening":
		state = "聞いています"
	case "thinking":
		state = "作業中"
	case "replying":
		if s.voiceAudio {
			state = "話しています"
		}
	}
	nameX := 196 + ib.Dx()*960/max(r.w, 1) + 14
	text(26, true, c.Name, nameX, 42, p.text)
	text(18, false, state, nameX, 66, p.muted)
	r.rightText(r.styleFace(false, 24), now.Format("15:04"), r.s(930), r.s(46), p.muted)

	y := 96
	// What was heard, on the right, in the owner's color.
	if s.heard != "" || s.phase == "listening" {
		f := r.styleFace(false, 24)
		lines := []string{"…"}
		if s.heard != "" {
			lines = r.wrap(f, s.heard, r.s(560))
			if len(lines) > 3 {
				lines = append(lines[:2], r.fit(f, lines[2]+" "+lines[len(lines)-1], r.s(560)))
			}
		}
		w := 0
		for _, l := range lines {
			w = max(w, r.width(f, l))
		}
		w += r.s(40)
		h := r.s(26 + 34*len(lines))
		b := image.Rect(r.s(936)-w, r.s(y), r.s(936), r.s(y)+h)
		r.roundFill(b, float64(r.s(20)), p.mine, p.mine)
		for i, l := range lines {
			r.text(f, l, b.Min.X+r.s(20), r.s(y+42+34*i), p.mineText)
		}
		if s.heard == "" {
			// Listening: three dots in place of words, on the 400 ms clock every dot here keeps.
			for i := 0; i < 3; i++ {
				col := color.RGBA{255, 255, 255, 120}
				if int(now.UnixMilli()/400%3) == i {
					col = p.mineText
				}
				r.roundFill(image.Rect(b.Min.X+r.s(20+i*18), r.s(y+30), b.Min.X+r.s(30+i*18), r.s(y+40)), float64(r.s(5)), col, col)
			}
			r.wantFrame(400*time.Millisecond - time.Duration(now.UnixMilli()%400)*time.Millisecond)
		}
		y += 26 + 34*len(lines) + 16
	}

	footer := "タップで会話を中断"
	switch {
	case s.reply != "":
		// The answer, on the left, with the character's face beside it. A long one turns pages.
		f := r.styleFace(false, 26)
		v := &r.turnPanel
		width := r.s(620)
		if v.text != s.reply || v.size != 26 || v.width != width {
			v.lines = r.wrap(f, s.reply, width)
			v.text, v.size, v.width = s.reply, 26, width
		}
		rows := max(1, (440-y-80)/38)
		pages := max(1, (len(v.lines)+rows-1)/rows)
		r.zmu.Lock()
		page := min(max(0, v.page), pages-1)
		v.page = page
		prev, next := rect(700, 420, 100, 40), rect(812, 420, 100, 40)
		if page > 0 {
			v.prev = prev
		}
		if page+1 < pages {
			v.next = next
		}
		r.zmu.Unlock()
		shown := v.lines[page*rows : min((page+1)*rows, len(v.lines))]
		w := 0
		for _, l := range shown {
			w = max(w, r.width(f, l))
		}
		w += r.s(44)
		h := r.s(26 + 38*len(shown))
		x0 := r.s(24) + ib.Dx() + r.s(12)
		b := image.Rect(x0, r.s(y), x0+w, r.s(y)+h)
		r.roundFill(b, float64(r.s(20)), p.theirs, p.theirs)
		draw.Draw(r.dst, ib.Add(image.Pt(r.s(24), b.Max.Y-ib.Dy())), sc.icon, ib.Min, draw.Over)
		for i, l := range shown {
			r.text(f, l, b.Min.X+r.s(22), r.s(y+44+38*i), p.text)
		}
		if pages > 1 {
			text(18, false, fmt.Sprintf("%d / %d", page+1, pages), 640, 447, p.muted)
			if page > 0 {
				r.roundFill(prev, float64(r.s(12)), p.pill, p.pill)
				text(20, true, "前へ", 728, 448, p.accent)
			}
			if page+1 < pages {
				r.roundFill(next, float64(r.s(12)), p.pill, p.pill)
				text(20, true, "次へ", 840, 448, p.accent)
			}
		}
		if s.phase == "lingering" {
			footer = "タップして話す"
		}
	case s.phase == "thinking" || s.phase == "replying":
		// At work: the busy picture, writing and now and then looking up, blinking; a thought
		// bubble of three dots over its head; and two lines, the second with the same dots.
		b := rect(24, max(y, 270), 600, 120)
		r.roundFill(b, float64(r.s(20)), p.theirs, p.theirs)
		bb := sc.busy.Bounds()
		at := image.Pt(b.Min.X+r.s(18), b.Max.Y-bb.Dy()-r.s(8)+dy)
		draw.Draw(r.dst, bb.Add(at), sc.busy, bb.Min, draw.Over)
		eyes, blinkNext := a.blink(now, false, false)
		r.wantFrame(blinkNext)
		if eyes != "" {
			drawPatch(r.dst, sc.busyExpr, exprEyesClosed, at, 0)
		} else if a.thinkingFace(now) {
			drawPatch(r.dst, sc.busyExpr, exprLookup, at, 0)
		}
		lit := int(now.UnixMilli() / 400 % 4)
		for i := 0; i < 3; i++ {
			col := p.line
			if i < lit {
				col = p.accent
			}
			cx, cy := at.X+bb.Dx()+r.s(2+i*8), at.Y-r.s(2+i*5)
			rad := r.s(3 + i)
			r.roundFill(image.Rect(cx-rad, cy-rad, cx+rad, cy+rad), float64(rad), col, col)
		}
		tx := at.X + bb.Dx() + r.s(32)
		r.text(r.styleFace(true, 26), c.Name+" が作業中", tx, b.Min.Y+r.s(52), p.text)
		dots := []string{"", "·", "··", "···"}[lit]
		r.text(r.styleFace(false, 20), "回答を準備しています"+dots, tx, b.Min.Y+r.s(86), p.muted)
		r.wantFrame(400*time.Millisecond - time.Duration(now.UnixMilli()%400)*time.Millisecond)
	}

	// The foot: what a tap does.
	fb := rect(24, 436, 912, 36)
	r.roundFill(fb, float64(r.s(12)), p.pill, p.pill)
	text(20, false, footer, 44, 461, p.accent)
}

// drawCharaSplash puts the character on the boot splash, above the name, and says who it is.
func (r *renderer) drawCharaSplash(c *charaSet, s *splash) {
	sc := c.scaled(r.w, r.h)
	pic := fitHeight(sc.idle[0], r.h*50/100)
	b := pic.Bounds()
	// The name is the splash's own, under the picture, with room left under it for the two lines
	// splashWaiting may add; the Muse sprite, if one is installed, gives way to the character.
	s.set, s.face = nil, r.big
	m := s.face.Metrics()
	rise, drop := m.CapHeight.Round(), m.Descent.Round()
	s.nameX = (r.w - r.width(s.face, markHey+markMuse)) / 2
	total := b.Dy() + r.s(markGapBase) + rise
	top := max(r.s(12), (r.h-total)/2-r.s(34))
	draw.Draw(r.dst, b.Add(image.Pt((r.w-b.Dx())/2, top)), pic, b.Min, draw.Over)
	s.baseline = top + total
	s.below = s.baseline + drop
	s.cx, s.cy = float64(r.w)/2, float64(top)+float64(b.Dy())/2
	s.from = float64(b.Dx())/2 + float64(r.s(arcClearBase))
}

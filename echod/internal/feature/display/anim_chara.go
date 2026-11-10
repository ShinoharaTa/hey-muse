//go:build !dot && !spot

package display

import (
	"math"
	"math/rand/v2"
	"time"
)

// The resident character's small motions (render_chara.go draws them). None of it is a loop that
// runs all day: a blink now and then, a slow breath by day, and a hop when something happens, which
// is what makes a still picture read as somebody there. At night the breathing stops and the blinks
// slow down, since a thing moving in the corner of the eye at three in the morning is not company.
//
// Timings are the motion direction's: a blink is half, closed, half at 80 ms each; blinks come 2 s
// plus an exponential 2 s apart, 15% of them doubled; a breath is 1.6 s in, 2.2 s out, 0.4 s held,
// the picture at most 0.8% taller at the top of it with the feet still.

// charaAnim is where the character is in its motions, kept by the renderer between frames.
type charaAnim struct {
	rng *rand.Rand

	// blinkAt is when the blink under way began, or when the next one begins; second is a doubled
	// blink's second half still to come.
	blinkAt time.Time
	second  bool

	// bounceAt and bounceKind are the hop under way: "listen", "reply" or "pet".
	bounceAt   time.Time
	bounceKind string

	// petAt is the last pat on the head, for its smile, its sparkles and its cooldown.
	petAt time.Time

	// said is a line put in the bubble for a moment (say), until saidUntil.
	said      string
	saidUntil time.Time

	// line and lineAt are the speech bubble's line and when it changed, for its rise.
	line   string
	lineAt time.Time

	// faceUp and faceUntil are the thinking card's face: writing, or looking up, and until when.
	faceUp    bool
	faceUntil time.Time

	// phase is the turn phase last drawn, so a change can fire a hop.
	phase string
}

const (
	blinkStep     = 80 * time.Millisecond
	blinkGapMean  = 2 * time.Second
	blinkGapMin   = 2 * time.Second
	blinkGapMax   = 8 * time.Second
	blinkNightMin = 6 * time.Second
	blinkNightMax = 12 * time.Second
	blinkDouble   = 0.15
	blinkSecondIn = 160 * time.Millisecond

	breathIn     = 1.6
	breathOut    = 2.2
	breathHold   = 0.4
	breathPeriod = breathIn + breathOut + breathHold

	petCooldown = 1500 * time.Millisecond
	petSmile    = 1200 * time.Millisecond
	petSparkle  = 700 * time.Millisecond
	bubbleRise  = 220 * time.Millisecond
)

func (a *charaAnim) rand() *rand.Rand {
	if a.rng == nil {
		a.rng = rand.New(rand.NewPCG(uint64(time.Now().UnixNano()), 0x706f6d))
	}
	return a.rng
}

// blinkGap is how long until the next blink.
func (a *charaAnim) blinkGap(night bool) time.Duration {
	r := a.rand()
	if night {
		return blinkNightMin + time.Duration(r.Float64()*float64(blinkNightMax-blinkNightMin))
	}
	gap := blinkGapMin + time.Duration(r.ExpFloat64()*float64(blinkGapMean))
	return min(gap, blinkGapMax)
}

// blink says which eyes to draw now: an expression name while a blink is under way, or "" between
// blinks, and when the next frame of the blink is due (0 between blinks). half is whether a half-open
// expression is there to use; without it the eyes close for two steps.
func (a *charaAnim) blink(now time.Time, night, half bool) (expr string, next time.Duration) {
	steps := 2
	if half {
		steps = 3
	}
	dur := time.Duration(steps) * blinkStep
	if a.blinkAt.IsZero() {
		a.blinkAt = now.Add(a.blinkGap(night))
	}
	if now.Before(a.blinkAt) {
		return "", 0
	}
	t := now.Sub(a.blinkAt)
	if t >= dur {
		// Over: the second of a double follows at once, else the next comes after a gap.
		if a.second {
			a.second = false
			a.blinkAt = a.blinkAt.Add(dur + blinkSecondIn)
		} else {
			a.second = !night && a.rand().Float64() < blinkDouble
			a.blinkAt = now.Add(a.blinkGap(night))
		}
		if now.Before(a.blinkAt) {
			return "", 0
		}
		t = now.Sub(a.blinkAt)
	}
	step := int(t / blinkStep)
	next = blinkStep - t%blinkStep
	if half && (step == 0 || step == 2) {
		return exprEyesHalf, next
	}
	return exprEyesClosed, next
}

// blinkSoon forces a blink now, the way eyes blink when something changes, unless one is under way.
func (a *charaAnim) blinkSoon(now time.Time) {
	if a.blinkAt.IsZero() || now.Before(a.blinkAt) || now.Sub(a.blinkAt) > 3*blinkStep {
		a.blinkAt = now
		a.second = false
	}
}

// untilBlink is how long until the next blink begins, for a frame to be ready for it.
func (a *charaAnim) untilBlink(now time.Time) time.Duration {
	if a.blinkAt.IsZero() {
		return 0
	}
	return max(a.blinkAt.Sub(now), time.Millisecond)
}

// breath is the breathing frame now, 0 to charaBreaths-1, from a 4.2 s cycle whose length drifts a
// little so it never ticks like a clock.
func breath(now time.Time) int {
	t := float64(now.UnixMilli()) / 1000
	t += 0.42 * math.Sin(t/37) // ±10% of a cycle, over a minute or so
	u := math.Mod(t, breathPeriod)
	var v float64
	switch {
	case u < breathIn:
		v = easeInOutSine(u / breathIn)
	case u < breathIn+breathOut:
		v = 1 - easeOutQuad((u-breathIn)/breathOut)
	default:
		v = 0
	}
	return int(v*float64(charaBreaths-1) + 0.5)
}

// hop starts a bounce of a kind, unless one of the same kind is under way.
func (a *charaAnim) hop(now time.Time, kind string) {
	a.bounceAt, a.bounceKind = now, kind
}

// bounce is the hop under way: how far up the picture sits (negative is up), which shape it is in
// ("" at rest, "squash" or "stretch" during a pat), and when the next frame is due (0 at rest).
func (a *charaAnim) bounce(now time.Time) (dy int, shape string, next time.Duration) {
	if a.bounceAt.IsZero() {
		return 0, "", 0
	}
	t := now.Sub(a.bounceAt)
	const frame = 80 * time.Millisecond
	switch a.bounceKind {
	case "listen": // up 3 in 150 ms, down in 200 ms
		switch {
		case t < 150*time.Millisecond:
			return -int(3*easeOutQuad(float64(t)/150e6) + 0.5), "", frame
		case t < 350*time.Millisecond:
			return -int(3*(1-easeInQuad(float64(t-150*time.Millisecond)/200e6)) + 0.5), "", frame
		}
	case "reply": // a 2 px nod over 300 ms
		if t < 300*time.Millisecond {
			return -int(2*math.Sin(math.Pi*float64(t)/300e6) + 0.5), "", frame
		}
	case "pet": // squash 80, stretch up 6 for 80, land over 160 with a little overshoot
		switch {
		case t < 80*time.Millisecond:
			return 0, "squash", frame
		case t < 160*time.Millisecond:
			return -6, "stretch", frame
		case t < 320*time.Millisecond:
			p := easeOutBack(float64(t-160*time.Millisecond) / 160e6)
			return -int(6*(1-p) - 0.5), "", frame
		}
	}
	a.bounceAt, a.bounceKind = time.Time{}, ""
	return 0, "", 0
}

// pet is a pat on the head: a hop, a smile, and sparkles, not more often than the cooldown.
func (a *charaAnim) pet(now time.Time) bool {
	if !a.petAt.IsZero() && now.Sub(a.petAt) < petCooldown {
		return false
	}
	a.petAt = now
	a.hop(now, "pet")
	a.blinkSoon(now.Add(petSmile + 160*time.Millisecond))
	return true
}

// petted is how far into the pat's smile and sparkles we are, 0 to 1, and whether it is on.
func (a *charaAnim) petted(now time.Time) (smile bool, sparkle float64, on bool) {
	if a.petAt.IsZero() {
		return false, 0, false
	}
	t := now.Sub(a.petAt) - 160*time.Millisecond
	if t < 0 {
		return false, 0, true
	}
	smile = t < petSmile
	if t < petSparkle {
		sparkle = float64(t) / float64(petSparkle)
	} else {
		sparkle = 1
	}
	return smile, sparkle, t < petSmile
}

// bubble notes the speech bubble's line and says how far the bubble has risen into place, 0 to 1.
func (a *charaAnim) bubble(now time.Time, line string) (rise float64, moving bool) {
	if line != a.line {
		a.line, a.lineAt = line, now
		a.blinkSoon(now)
	}
	t := now.Sub(a.lineAt)
	if t >= bubbleRise {
		return 1, false
	}
	return easeOutCubic(float64(t) / float64(bubbleRise)), true
}

// thinkingFace is the face on the busy card: writing (false) or looking up (true), writing for 2.5 to
// 4 s at a time and looking up for 1.2 s.
func (a *charaAnim) thinkingFace(now time.Time) bool {
	if now.After(a.faceUntil) {
		a.faceUp = !a.faceUp
		if a.faceUp {
			a.faceUntil = now.Add(1200 * time.Millisecond)
		} else {
			a.faceUntil = now.Add(2500*time.Millisecond + time.Duration(a.rand().Float64()*1500e6))
		}
	}
	return a.faceUp
}

// turnPhase notes the turn's phase and fires what a change brings: a hop and a blink on listening and
// replying, a fresh face on thinking.
func (a *charaAnim) turnPhase(now time.Time, phase string) {
	if phase == a.phase {
		return
	}
	a.phase = phase
	switch phase {
	case "listening":
		a.hop(now, "listen")
		a.blinkSoon(now)
	case "replying":
		a.hop(now, "reply")
		a.blinkSoon(now)
	case "thinking":
		a.faceUp, a.faceUntil = true, time.Time{}
	}
}

func easeInQuad(t float64) float64    { return t * t }
func easeOutQuad(t float64) float64   { return 1 - (1-t)*(1-t) }
func easeOutCubic(t float64) float64  { return 1 - math.Pow(1-t, 3) }
func easeInOutSine(t float64) float64 { return -(math.Cos(math.Pi*t) - 1) / 2 }
func easeOutBack(t float64) float64 {
	const c1, c3 = 1.2, 2.2 // weaker than the usual 1.70158: a chibi lands, it does not spring
	return 1 + c3*math.Pow(t-1, 3) + c1*math.Pow(t-1, 2)
}

// say puts a line in the bubble for a moment, over the line of the hour: the name of a layout a swipe
// turned to.
func (a *charaAnim) say(line string, now time.Time) {
	a.said, a.saidUntil = line, now.Add(1500*time.Millisecond)
}

// saying is the line say put up, while it is due to stay.
func (a *charaAnim) saying(now time.Time) string {
	if now.Before(a.saidUntil) {
		return a.said
	}
	return ""
}

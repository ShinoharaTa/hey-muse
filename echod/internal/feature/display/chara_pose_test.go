//go:build !dot && !spot

package display

import (
	"image"
	"testing"
	"time"
)

// TestPickPoseKeepsToTheHourAndTheHalfHour picks the same pose all through a half hour, only poses
// done at that time of day, and standing when none fits.
func TestPickPoseKeepsToTheHourAndTheHalfHour(t *testing.T) {
	poses := []charaPose{{Name: "study", Hours: []string{roomDay}}, {Name: "coffee", Hours: []string{roomMorning, roomDay}}, {Name: "relax"}}
	all := func(charaPose) bool { return true }
	at := time.Date(2026, 11, 10, 13, 0, 0, 0, time.Local)
	first := pickPose(poses, at, roomDay, all)
	for m := range 30 {
		if got := pickPose(poses, at.Add(time.Duration(m)*time.Minute), roomDay, all); got != first {
			t.Fatalf("13:%02d picked %q, 13:00 %q", m, got, first)
		}
	}
	seen := map[string]bool{}
	for h := range 96 {
		seen[pickPose(poses, at.Add(time.Duration(h)*charaPoseEvery), roomNight, all)] = true
	}
	if seen["study"] || seen["coffee"] || !seen["relax"] || !seen[""] {
		t.Errorf("at night picked %v, want relax and standing only", seen)
	}
	none := func(charaPose) bool { return false }
	if got := pickPose(poses, at, roomDay, none); got != "" {
		t.Errorf("nothing fits, picked %q", got)
	}
}

// TestScalePoseLandsOnItsFurniture puts a pose where its spot in the room picture lands on the screen:
// the room 1536x1024 fills 960 wide, so it is scaled by 0.625 and its middle band shown.
func TestScalePoseLandsOnItsFurniture(t *testing.T) {
	src := image.NewRGBA(image.Rect(0, 0, 200, 400))
	ps := scalePose(charaPose{X: 1262, Y: 870, H: 400}, src, charaPatch{}, image.Pt(1536, 1024), 960, 480)
	b := ps.rect()
	if b.Dy() != 250 || b.Dx() != 125 {
		t.Fatalf("scaled to %v, want 125x250", b.Size())
	}
	if mid := (b.Min.X + b.Max.X) / 2; mid < 787 || mid > 790 {
		t.Errorf("middle at x=%d, want about 789", mid)
	}
	if b.Max.Y < 463 || b.Max.Y > 465 {
		t.Errorf("bottom at y=%d, want about 464", b.Max.Y)
	}
	r := newRenderer(image.NewRGBA(image.Rect(0, 0, 960, 480)))
	if !r.poseFree(ps, layoutStandard) || !r.poseFree(ps, layoutClock) || r.poseFree(ps, layoutChara) {
		t.Error("a pose at the right should fit the standard and clock layouts, never the large character's")
	}
	left := scalePose(charaPose{X: 900, Y: 870, H: 400}, src, charaPatch{}, image.Pt(1536, 1024), 960, 480)
	if r.poseFree(left, layoutStandard) || !r.poseFree(left, layoutClock) {
		t.Error("a pose under the tiles should fit only the clock layout")
	}
}

// TestCalledToTheFront stands the character up after a pat and a turn, for charaStandFor.
func TestCalledToTheFront(t *testing.T) {
	now := time.Date(2026, 11, 10, 13, 0, 0, 0, time.Local)
	var a charaAnim
	a.pet(now)
	if !a.standUntil.Equal(now.Add(charaStandFor)) {
		t.Errorf("after a pat, standing until %v", a.standUntil)
	}
	a.turnPhase(now.Add(time.Hour), "listening")
	if !a.standUntil.Equal(now.Add(time.Hour + charaStandFor)) {
		t.Errorf("after a turn, standing until %v", a.standUntil)
	}
	if was, in := a.pose(now, "study"); was != "" || in != 0 {
		t.Errorf("a new pose starts its fade from standing: %q %v", was, in)
	}
	if was, in := a.pose(now.Add(charaPoseFade), "study"); was != "study" || in != 1 {
		t.Errorf("after the fade: %q %v", was, in)
	}
}

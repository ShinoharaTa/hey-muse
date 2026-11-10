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

// TestTintFollowsTheRoom dims a picture by three quarters of how much darker the room is than the day
// room, not below half, and leaves the day room's light alone.
func TestTintFollowsTheRoom(t *testing.T) {
	day := [3]float64{189, 158, 171}
	if got := tintFor(day, day); got != [3]float64{1, 1, 1} {
		t.Errorf("the day room tints %v", got)
	}
	dawn := tintFor([3]float64{86, 75, 124}, day)
	if dawn[0] < 0.58 || dawn[0] > 0.6 || dawn[2] < 0.78 || dawn[2] > 0.8 {
		t.Errorf("dawn tints %v, want about 0.59 red and 0.79 blue", dawn)
	}
	if black := tintFor([3]float64{}, day); black != [3]float64{tintFloor, tintFloor, tintFloor} {
		t.Errorf("a black room tints %v", black)
	}
	src := image.NewRGBA(image.Rect(0, 0, 1, 1))
	copy(src.Pix, []uint8{200, 100, 50, 200})
	out := tinted(src, [3]float64{0.5, 1, 0.5})
	if got := out.Pix; got[0] != 100 || got[1] != 100 || got[2] != 25 || got[3] != 200 || src.Pix[0] != 200 {
		t.Errorf("tinted %v (source now %v)", got, src.Pix)
	}
}

// TestCharaPosesPreview draws each pose of a real character set in two rooms and both layouts that
// have room for poses: CHARA_DIR names the set and SHOW_PREVIEW where the PNGs go.
func TestCharaPosesPreview(t *testing.T) {
	src, out := os.Getenv("CHARA_DIR"), os.Getenv("SHOW_PREVIEW")
	if src == "" || out == "" {
		t.Skip("CHARA_DIR and SHOW_PREVIEW not set")
	}
	c, err := loadCharaFrom(src)
	if err != nil {
		t.Fatal(err)
	}
	defer func(f func(time.Time) (time.Time, time.Time, bool)) { charaRoomSun = f }(charaRoomSun)
	day := time.Date(2026, 11, 10, 0, 0, 0, 0, time.Local)
	charaRoomSun = func(time.Time) (time.Time, time.Time, bool) {
		return day.Add(6*time.Hour + 13*time.Minute), day.Add(16*time.Hour + 35*time.Minute), true
	}
	all := c.Poses
	for _, p := range all {
		p.Hours = nil
		c.Poses = []charaPose{p}
		for _, when := range []struct {
			name string
			off  time.Duration
			cond string
		}{{"day", 13 * time.Hour, "sunny"}, {"night", 21 * time.Hour, "rainy"}} {
			at := day.Add(when.off)
			for pickPose(c.Poses, at, roomDay, func(charaPose) bool { return true }) != p.Name {
				at = at.Add(charaPoseEvery)
			}
			for _, layout := range []string{layoutStandard, layoutClock} {
				pinCharaView(t, charaView{Layout: layout})
				img := image.NewRGBA(image.Rect(0, 0, showWide, showHigh))
				r := newRenderer(img)
				for range 500 { // the room and the pose are scaled off the frame
					want := c.roomFile(roomHour(at, day.Add(6*time.Hour+13*time.Minute), day.Add(16*time.Hour+35*time.Minute), true), roomSky(when.cond))
					c.roomAt(showWide, showHigh, at, when.cond)
					if ps := c.poseScaled(p.Name, showWide, showHigh); ps != nil && c.roomShown() == want {
						break
					}
					time.Sleep(10 * time.Millisecond)
				}
				s := scene{now: at, phase: "idle", chara: c, weather: home.Weather{Condition: when.cond, Temp: "14°"},
					forecast: []hass.Day{{When: at, Condition: when.cond, High: 18, Low: 9}, {When: at.AddDate(0, 0, 1), Condition: "sunny", High: 19, Low: 10}}}
				r.draw(s)
				r.draw(s) // past the cross-fade from standing
				s.now = s.now.Add(time.Second)
				r.draw(s)
				f, err := os.Create(filepath.Join(out, "pose-"+p.Name+"-"+when.name+"-"+layout+".png"))
				if err != nil {
					t.Fatal(err)
				}
				png.Encode(f, img)
				f.Close()
			}
		}
	}
	c.Poses = all
}

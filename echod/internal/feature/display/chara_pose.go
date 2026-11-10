//go:build !dot && !spot

package display

import (
	"image"
	"image/png"
	"log/slog"
	"math/rand/v2"
	"os"
	"path/filepath"
	"slices"
	"time"

	"github.com/HuskerMinion/techo5/echod/internal/lib/safe"
)

// The character's ways of spending the day in the room: optional pictures of it sitting at the desk,
// with a coffee, lazing on a cushion, each drawn at the room's scale where the furniture is in the
// room's picture. chara.json says where:
//
//	"poses": [{"name": "study", "x": 1262, "y": 870, "h": 420, "hours": ["day", "night"],
//	           "lines": ["いま勉強中…"]}]
//
// name is the picture, pose_study.png (and pose_study_eyes_closed.png for its blink); x and y are the
// middle of the picture's bottom edge and h its height, in the room picture's own pixels; hours are the
// times of day it is done (chara_room.go), all of them when left out; lines replace the bubble's while
// it is done. Every half hour one is picked, or standing, from those that fit the hour and the layout.
// A pat on the head, a turn, or a layout without room for the pose brings the character to the front,
// standing, for charaStandFor.

// charaPose is one entry of chara.json's poses.
type charaPose struct {
	Name  string   `json:"name"`
	X     float64  `json:"x"`
	Y     float64  `json:"y"`
	H     float64  `json:"h"`
	Hours []string `json:"hours"`
	Lines []string `json:"lines"`
}

const (
	// charaPoseEvery is how long one pick lasts.
	charaPoseEvery = 30 * time.Minute
	// charaStandFor is how long the character stays standing at the front after it is called.
	charaStandFor = 3 * time.Minute
	// charaPoseFade is the cross-fade from one pose to another.
	charaPoseFade = 300 * time.Millisecond
)

// exprPoseEyes is a pose's blink: the eyes may be anywhere in its upper part.
const exprPoseEyes = "pose_eyes_closed"

// charaPoseScaled is a pose at the screen's size, where it sits on the screen.
type charaPoseScaled struct {
	img  *image.RGBA
	at   image.Point
	eyes charaPatch // the blink, img nil without one
}

func (p *charaPoseScaled) rect() image.Rectangle { return p.img.Bounds().Add(p.at) }

// loadPoses reads the pictures of the poses chara.json names, dropping one without its picture.
func (c *charaSet) loadPoses(dir string) {
	c.poseImg, c.posePatch = map[string]image.Image{}, map[string]charaPatch{}
	var kept []charaPose
	for _, p := range c.Poses {
		img, err := readPNG(filepath.Join(dir, "pose_"+p.Name+".png"))
		if err != nil || p.H <= 0 {
			slog.Warn("character pose not loaded", "pose", p.Name, "err", err, "h", p.H)
			continue
		}
		c.poseImg[p.Name] = img
		if eyes, err := readPNG(filepath.Join(dir, "pose_"+p.Name+"_eyes_closed.png")); err == nil {
			if patch, err := facePatch(img, eyes, exprPoseEyes); err == nil {
				c.posePatch[p.Name] = patch
			} else {
				slog.Warn("character pose blink dropped", "pose", p.Name, "err", err)
			}
		}
		kept = append(kept, p)
	}
	c.Poses = kept
	if len(kept) == 0 {
		return
	}
	// Where the poses are is in the room picture's pixels, so its size is needed to place them.
	if c.bg != nil {
		c.roomSize = c.bg.Bounds().Size()
		return
	}
	for name := range c.rooms {
		if f, err := os.Open(filepath.Join(dir, charaRoomsDir, name)); err == nil {
			cfg, err := png.DecodeConfig(f)
			f.Close()
			if err == nil {
				c.roomSize = image.Pt(cfg.Width, cfg.Height)
				return
			}
		}
	}
	slog.Warn("character poses need a room picture to be placed in", "poses", len(kept))
	c.Poses = nil
}

// poseScaled is a pose at a w by h screen, or nil while it is still being scaled, off the frame.
func (c *charaSet) poseScaled(name string, w, h int) *charaPoseScaled {
	size := image.Pt(w, h)
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.poseSize != size {
		c.poseSize, c.poseCache = size, map[string]*charaPoseScaled{}
	}
	if ps := c.poseCache[name]; ps != nil {
		return ps
	}
	if c.posePending == nil {
		c.posePending = map[string]bool{}
	}
	i := slices.IndexFunc(c.Poses, func(p charaPose) bool { return p.Name == name })
	if i < 0 || c.posePending[name] {
		return nil
	}
	c.posePending[name] = true
	p, src, patch := c.Poses[i], c.poseImg[name], c.posePatch[name]
	safe.Go("chara pose", func() {
		ps := scalePose(p, src, patch, c.roomSize, w, h)
		c.mu.Lock()
		defer c.mu.Unlock()
		delete(c.posePending, name)
		if c.poseSize == size {
			c.poseCache[name] = ps
		}
	})
	return nil
}

// scalePose fits a pose to a w by h screen the way coverScale fits the room, so it lands on its furniture.
func scalePose(p charaPose, src image.Image, patch charaPatch, room image.Point, w, h int) *charaPoseScaled {
	sw, sh := float64(room.X), float64(room.Y)
	scale := max(float64(w)/sw, float64(h)/sh)
	x0, y0 := (sw-float64(w)/scale)/2, (sh-float64(h)/scale)/2
	img := fitHeight(src, int(p.H*scale+0.5))
	ps := &charaPoseScaled{img: img, at: image.Pt(int((p.X-x0)*scale+0.5)-img.Bounds().Dx()/2, int((p.Y-y0)*scale+0.5)-img.Bounds().Dy())}
	if patch.img != nil {
		ps.eyes = scalePatch(patch, src.Bounds(), img.Bounds())
	}
	return ps
}

// poseFree is whether a pose on the screen is clear of a layout's clock, tiles and buttons: the large
// character's layout has none, the standard one keeps the left 610 pixels, the clock's 480.
func (r *renderer) poseFree(ps *charaPoseScaled, layout string) bool {
	var left int
	switch layout {
	case layoutChara:
		return false
	case layoutClock:
		left = r.s(480)
	default:
		left = r.s(610)
	}
	b := ps.rect()
	return b.Min.X >= left && b.Max.X <= r.w+r.s(20)
}

// pickPose is the pose for now's half hour: one of those that may be done at this time of day and fit
// (ok), or standing (""), the same pick all through the half hour.
func pickPose(poses []charaPose, now time.Time, hour string, ok func(charaPose) bool) string {
	names := []string{""}
	for _, p := range poses {
		if (len(p.Hours) == 0 || slices.Contains(p.Hours, hour)) && ok(p) {
			names = append(names, p.Name)
		}
	}
	slot := uint64(now.Unix() / int64(charaPoseEvery/time.Second))
	return names[rand.New(rand.NewPCG(slot, 0x706f6d)).IntN(len(names))]
}

// poseNow is the pose to draw now in a layout, with it scaled, or "" and nil for standing: standing
// while the character has been called to the front, and while the pose is still being scaled.
func (r *renderer) poseNow(c *charaSet, now time.Time, layout string) (string, *charaPoseScaled) {
	if len(c.Poses) == 0 || now.Before(r.chara.standUntil) || layout == layoutChara {
		return "", nil
	}
	rise, set, ok := charaRoomSun(now)
	hour := roomHour(now, rise, set, ok)
	name := pickPose(c.Poses, now, hour, func(p charaPose) bool {
		ps := c.poseScaled(p.Name, r.w, r.h)
		return ps != nil && r.poseFree(ps, layout)
	})
	if name == "" {
		return "", nil
	}
	return name, c.poseScaled(name, r.w, r.h)
}

// poseLine is the bubble's line while a pose is done: its own, or the ordinary one.
func (c *charaSet) poseLine(name string, now time.Time) string {
	for _, p := range c.Poses {
		if p.Name == name && len(p.Lines) > 0 {
			return p.Lines[int(now.Unix()/int64(charaLineEvery/time.Second))%len(p.Lines)]
		}
	}
	return c.charaLine(now)
}

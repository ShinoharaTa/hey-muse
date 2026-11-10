//go:build !dot && !spot

package display

import (
	"encoding/json"
	"errors"
	"fmt"
	"image"
	"image/color"
	"image/draw"
	"image/png"
	"log/slog"
	"os"
	"path/filepath"
	"sync"
	"sync/atomic"

	xdraw "golang.org/x/image/draw"

	"github.com/HuskerMinion/techo5/echod/internal/layout"
)

// The resident character: the owner's own picture of who answers, drawn on the idle page all day
// (render_chara.go) rather than only while a turn runs, the way the Muse sprite is. It is a few PNGs
// and a small JSON in charaDir, put there by the owner; nothing is built in. With none there the
// screen looks as it always has.
//
//	chara.json            {"name": "ポム", "lines": ["ねえ、何か用事ある？"], "place": "東京"}
//	idle.png              the character standing, on a transparent background (required)
//	busy.png              the character at work, for "thinking" (optional: idle is used)
//	background.png        the room behind everything on the idle page (optional: frosted glass instead)
//	rooms/*.png           the room by time of day and weather, in background.png's place (chara_room.go)
//
// Expressions are optional edits of those two pictures, same pose and framing, with only the face
// changed (anim_chara.go says when each shows):
//
//	idle_eyes_half.png    eyelids halfway down, the middle of a blink
//	idle_eyes_closed.png  eyes closed, the blink itself
//	idle_happy.png        the smile for a pat on the head
//	busy_eyes_closed.png  the blink while at work
//	busy_lookup.png       looking up, pondering
//
// Only the face is kept from an edit: the pixels that differ from the base, inside a box around the
// face, feathered at the edge. An edit that moved the pose is dropped, since a face that jumps is
// worse than one that never blinks.
const charaDir = layout.StateDir + "/chara"

// charaMaxPixels caps a picture: a 1 GB device should not decode a poster.
const charaMaxPixels = 4000 * 4000

// charaConfig is chara.json.
type charaConfig struct {
	// Name is what the character is called on the turn page's header and busy card.
	Name string `json:"name"`
	// Lines are what the speech bubble on the idle page says, one at a time, changing every few minutes.
	Lines []string `json:"lines"`
	// Petted are the lines for a pat on the head; empty keeps the ordinary line.
	Petted []string `json:"petted"`
	// Place names the weather tiles ("今日・東京"); empty leaves the place off.
	Place string `json:"place"`
	// Hello is the line under the name on the boot splash; empty leaves it off.
	Hello string `json:"hello"`
	// Poses are the ways the character spends the day in the room (chara_pose.go).
	Poses []charaPose `json:"poses"`
}

// Expression names, as the files are named.
const (
	exprEyesHalf   = "eyes_half"
	exprEyesClosed = "eyes_closed"
	exprHappy      = "happy"
	exprLookup     = "lookup"
)

// charaFaces is which box of a picture an expression may change, in the picture's own pixels as a
// share of its width and height: eyes high in the face for a blink, the whole face for a smile.
var charaFaces = map[string]struct{ x0, y0, x1, y1 float64 }{
	exprEyesHalf:   {0.15, 0.26, 0.85, 0.56},
	exprEyesClosed: {0.15, 0.26, 0.85, 0.56},
	exprHappy:      {0.12, 0.22, 0.88, 0.62},
	exprLookup:     {0.12, 0.22, 0.88, 0.62},
	exprPoseEyes:   {0, 0, 1, 0.7},
}

// charaPatch is an expression at the picture's own size: the changed face, feathered, and where it
// sits on the base.
type charaPatch struct {
	img *image.RGBA
	at  image.Point
}

// charaSet is the loaded character: the pictures as decoded, and scaled copies per screen size.
type charaSet struct {
	charaConfig
	bg, idle, busy image.Image
	idlePatch      map[string]charaPatch // by expression
	busyPatch      map[string]charaPatch

	dir   string          // where the pictures were read from
	rooms map[string]bool // the room pictures there (chara_room.go)

	poseImg   map[string]image.Image // the poses as decoded, by name (chara_pose.go)
	posePatch map[string]charaPatch  // their blinks
	roomSize  image.Point            // the room picture's size, which the poses are placed in

	mu          sync.Mutex
	cache       map[image.Point]*charaScaled
	pending     map[image.Point]bool // sizes scaledSoon is making
	room        charaRoom            // the room in use
	roomPending string               // the room being scaled to replace it
	dayMean     [3]float64           // the clear day room's average color, once dayMeasured
	dayMeasured bool
	lit         map[string]*image.RGBA // pictures in the room's light (litIn), for the room in use
	poseSize    image.Point            // the screen size poseCache is for
	poseCache   map[string]*charaPoseScaled
	posePending map[string]bool
}

// charaBreaths is how many breathing frames the standing picture has: each a little taller than the
// last, the feet staying put. The last is charaBreathMax taller.
const (
	charaBreaths   = 5
	charaBreathMax = 0.008
)

// charaScaled is the set fitted to one screen size.
type charaScaled struct {
	bg *image.RGBA // the whole screen, or nil
	// idle is standing, by breathing frame: idle[0] at rest. squash and stretch are the two shapes a
	// pat on the head goes through. All are drawn with their bottom on the same floor.
	idle            [charaBreaths]*image.RGBA
	squash, stretch *image.RGBA
	busy            *image.RGBA // at work, for the busy card
	icon            *image.RGBA // small, for the turn page's header and reply bubbles
	// Expressions, scaled to idle[0] and busy: drawn over the picture at their offset.
	idleExpr map[string]charaPatch
	busyExpr map[string]charaPatch
}

// charaCurrent is the character the device has, for pages drawn outside a scene (the boot splash).
var charaCurrent atomic.Pointer[charaSet]

// loadChara reads charaDir and says what it found. A missing directory is the usual case and is quiet;
// a broken one is said in the log, since the owner put it there expecting a character.
func loadChara() *charaSet {
	set, err := loadCharaFrom(charaDir)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			slog.Info("no resident character installed", "dir", charaDir)
		} else {
			slog.Warn("resident character not loaded", "dir", charaDir, "err", err)
		}
		return nil
	}
	slog.Info("resident character loaded", "dir", charaDir, "name", set.Name, "lines", len(set.Lines),
		"background", set.bg != nil, "rooms", len(set.rooms), "poses", len(set.Poses), "busy", set.busy != nil, "idle_expressions", len(set.idlePatch), "busy_expressions", len(set.busyPatch))
	charaCurrent.Store(set)
	return set
}

func loadCharaFrom(dir string) (*charaSet, error) {
	set := &charaSet{dir: dir, rooms: findRooms(dir), cache: map[image.Point]*charaScaled{}, idlePatch: map[string]charaPatch{}, busyPatch: map[string]charaPatch{}}
	if raw, err := os.ReadFile(filepath.Join(dir, "chara.json")); err == nil {
		if err := json.Unmarshal(raw, &set.charaConfig); err != nil {
			return nil, fmt.Errorf("chara.json: %w", err)
		}
	} else if !errors.Is(err, os.ErrNotExist) {
		return nil, err
	}
	var err error
	if set.idle, err = readPNG(filepath.Join(dir, "idle.png")); err != nil {
		return nil, fmt.Errorf("idle.png: %w", err)
	}
	if set.busy, err = readPNG(filepath.Join(dir, "busy.png")); err != nil && !errors.Is(err, os.ErrNotExist) {
		return nil, fmt.Errorf("busy.png: %w", err)
	}
	if set.bg, err = readPNG(filepath.Join(dir, "background.png")); err != nil && !errors.Is(err, os.ErrNotExist) {
		return nil, fmt.Errorf("background.png: %w", err)
	}
	if set.Name == "" {
		set.Name = "Muse"
	}
	set.loadPoses(dir)
	for _, e := range []string{exprEyesHalf, exprEyesClosed, exprHappy} {
		set.loadExpression(dir, "idle_"+e+".png", e, set.idle, set.idlePatch)
	}
	if set.busy != nil {
		for _, e := range []string{exprEyesClosed, exprLookup} {
			set.loadExpression(dir, "busy_"+e+".png", e, set.busy, set.busyPatch)
		}
	}
	return set, nil
}

// loadExpression reads one edit and keeps its face, or says why it could not.
func (c *charaSet) loadExpression(dir, file, name string, base image.Image, into map[string]charaPatch) {
	img, err := readPNG(filepath.Join(dir, file))
	if err != nil {
		if !errors.Is(err, os.ErrNotExist) {
			slog.Warn("character expression not loaded", "file", file, "err", err)
		}
		return
	}
	p, err := facePatch(base, img, name)
	if err != nil {
		slog.Warn("character expression dropped", "file", file, "err", err)
		return
	}
	into[name] = p
}

func readPNG(path string) (image.Image, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	cfg, err := png.DecodeConfig(f)
	if err != nil {
		return nil, err
	}
	if cfg.Width < 1 || cfg.Height < 1 || cfg.Width*cfg.Height > charaMaxPixels {
		return nil, fmt.Errorf("%dx%d pixels is not a picture this device draws", cfg.Width, cfg.Height)
	}
	if _, err := f.Seek(0, 0); err != nil {
		return nil, err
	}
	return png.Decode(f)
}

// Patch tolerances, in the picture's own pixels.
const (
	patchThreshold = 40 // a channel has to move this much for a pixel to count as changed
	patchPad       = 8  // the changed box grows this much
	patchFeather   = 6  // and fades to nothing over this much at its edge
)

// facePatch keeps the face from an edit: the box of pixels that differ from the base, grown and
// feathered. The box has to be inside the face's region for the expression, and the picture outside
// the region has to be the same, or the edit is refused.
func facePatch(base, edit image.Image, name string) (charaPatch, error) {
	b := base.Bounds()
	if edit.Bounds().Size() != b.Size() {
		return charaPatch{}, fmt.Errorf("%v is not the base's %v", edit.Bounds().Size(), b.Size())
	}
	bi, ei := toRGBA(base), toRGBA(edit)
	f := charaFaces[name]
	w, h := float64(b.Dx()), float64(b.Dy())
	region := image.Rect(int(f.x0*w), int(f.y0*h), int(f.x1*w), int(f.y1*h))
	var box image.Rectangle
	outside := 0
	for y := 0; y < b.Dy(); y++ {
		for x := 0; x < b.Dx(); x++ {
			i := y*bi.Stride + x*4
			if !changed(bi.Pix[i:i+4], ei.Pix[i:i+4]) {
				continue
			}
			p := image.Pt(x, y)
			if !p.In(region) {
				outside++
				continue
			}
			box = box.Union(image.Rect(x, y, x+1, y+1))
		}
	}
	if box.Empty() {
		return charaPatch{}, errors.New("nothing in the face differs from the base")
	}
	if outside > region.Dx()*region.Dy()/50 {
		return charaPatch{}, fmt.Errorf("%d pixels changed outside the face: the pose moved", outside)
	}
	box = box.Inset(-patchPad).Intersect(b.Sub(b.Min))
	out := image.NewRGBA(image.Rect(0, 0, box.Dx(), box.Dy()))
	for y := 0; y < box.Dy(); y++ {
		for x := 0; x < box.Dx(); x++ {
			// Alpha falls off toward the edge of the box, so the face sits in without a seam.
			edge := min(x, y, box.Dx()-1-x, box.Dy()-1-y)
			a := 1.0
			if edge < patchFeather {
				a = float64(edge+1) / float64(patchFeather+1)
			}
			si := (box.Min.Y+y)*ei.Stride + (box.Min.X+x)*4
			di := y*out.Stride + x*4
			for k := 0; k < 4; k++ {
				out.Pix[di+k] = uint8(float64(ei.Pix[si+k])*a + 0.5)
			}
		}
	}
	return charaPatch{img: out, at: box.Min}, nil
}

func changed(a, b []uint8) bool {
	for k := 0; k < 4; k++ {
		d := int(a[k]) - int(b[k])
		if d > patchThreshold || d < -patchThreshold {
			return true
		}
	}
	return false
}

func toRGBA(img image.Image) *image.RGBA {
	if r, ok := img.(*image.RGBA); ok && r.Rect.Min == (image.Point{}) {
		return r
	}
	out := image.NewRGBA(image.Rect(0, 0, img.Bounds().Dx(), img.Bounds().Dy()))
	draw.Draw(out, out.Rect, img, img.Bounds().Min, draw.Src)
	return out
}

// scaled is the set fitted to a w by h screen, made the first time that size is asked for.
func (c *charaSet) scaled(w, h int) *charaScaled {
	key := image.Pt(w, h)
	c.mu.Lock()
	s := c.cache[key]
	c.mu.Unlock()
	if s != nil {
		return s
	}
	return c.keep(key, c.build(w, h, true))
}

// scaledSoon is the character alone (no room) at a size a layout draws it, or nil while it is still
// being made: the first ask starts the scaling, which takes a second or two on the device, off the
// frame that asked.
func (c *charaSet) scaledSoon(w, h int) *charaScaled {
	key := image.Pt(w, h)
	c.mu.Lock()
	defer c.mu.Unlock()
	if s := c.cache[key]; s != nil {
		return s
	}
	if c.pending == nil {
		c.pending = map[image.Point]bool{}
	}
	if !c.pending[key] {
		c.pending[key] = true
		go func() {
			c.keep(key, c.build(w, h, false))
			c.mu.Lock()
			delete(c.pending, key)
			c.mu.Unlock()
		}()
	}
	return nil
}

// keep stores a scaled set, unless another was stored for the size meanwhile, and returns the one kept.
func (c *charaSet) keep(key image.Point, s *charaScaled) *charaScaled {
	c.mu.Lock()
	defer c.mu.Unlock()
	if old := c.cache[key]; old != nil {
		return old
	}
	if len(c.cache) > 6 {
		clear(c.cache)
	}
	c.cache[key] = s
	return s
}

// build scales the set to a w by h screen, the room too when room is set.
func (c *charaSet) build(w, h int, room bool) *charaScaled {
	s := &charaScaled{idleExpr: map[string]charaPatch{}, busyExpr: map[string]charaPatch{}}
	if c.bg != nil && room {
		s.bg = coverScale(c.bg, w, h)
	}
	standing := h * 86 / 100
	for i := range charaBreaths {
		s.idle[i] = fitHeight(c.idle, int(float64(standing)*(1+charaBreathMax*float64(i)/float64(charaBreaths-1))+0.5))
	}
	s.squash = scaleTo(c.idle, s.idle[0].Bounds().Dx()*102/100, standing*97/100)
	s.stretch = scaleTo(c.idle, s.idle[0].Bounds().Dx(), standing*102/100)
	busy := c.busy
	if busy == nil {
		busy = c.idle
	}
	s.busy = fitHeight(busy, h*24/100)
	s.icon = fitHeight(c.idle, h*11/100)
	for name, p := range c.idlePatch {
		s.idleExpr[name] = scalePatch(p, c.idle.Bounds(), s.idle[0].Bounds())
	}
	if c.busy != nil {
		for name, p := range c.busyPatch {
			s.busyExpr[name] = scalePatch(p, c.busy.Bounds(), s.busy.Bounds())
		}
	}
	return s
}

// scalePatch fits a face to the picture's scaled size, rounding its box outward so no edge is lost.
func scalePatch(p charaPatch, from, to image.Rectangle) charaPatch {
	sx := float64(to.Dx()) / float64(from.Dx())
	sy := float64(to.Dy()) / float64(from.Dy())
	b := p.img.Bounds()
	w := max(int(float64(b.Dx())*sx+0.5), 1)
	h := max(int(float64(b.Dy())*sy+0.5), 1)
	return charaPatch{img: scaleTo(p.img, w, h), at: image.Pt(int(float64(p.at.X)*sx+0.5), int(float64(p.at.Y)*sy+0.5))}
}

// scaleTo scales a picture to exactly w by h.
func scaleTo(src image.Image, w, h int) *image.RGBA {
	dst := image.NewRGBA(image.Rect(0, 0, max(w, 1), max(h, 1)))
	xdraw.CatmullRom.Scale(dst, dst.Bounds(), src, src.Bounds(), xdraw.Src, nil)
	return dst
}

// fitHeight scales a picture to h high, keeping its shape.
func fitHeight(src image.Image, h int) *image.RGBA {
	b := src.Bounds()
	h = max(h, 1)
	w := max((b.Dx()*h+b.Dy()/2)/max(b.Dy(), 1), 1)
	return scaleTo(src, w, h)
}

// coverScale scales a picture to fill w by h, cropping what sticks out, centered.
func coverScale(src image.Image, w, h int) *image.RGBA {
	b := src.Bounds()
	sw, sh := float64(b.Dx()), float64(b.Dy())
	scale := max(float64(w)/sw, float64(h)/sh)
	cw, ch := float64(w)/scale, float64(h)/scale
	x0 := float64(b.Min.X) + (sw-cw)/2
	y0 := float64(b.Min.Y) + (sh-ch)/2
	crop := image.Rect(int(x0), int(y0), int(x0+cw+0.5), int(y0+ch+0.5)).Intersect(b)
	dst := image.NewRGBA(image.Rect(0, 0, w, h))
	xdraw.CatmullRom.Scale(dst, dst.Bounds(), src, crop, xdraw.Src, nil)
	return dst
}

// drawPatch lays an expression over a picture drawn at `at`, shifted by dy (a taller breathing frame
// lifts the face). Nothing is drawn with no such expression.
func drawPatch(dst *image.RGBA, patches map[string]charaPatch, name string, at image.Point, dy int) bool {
	p, ok := patches[name]
	if !ok {
		return false
	}
	b := p.img.Bounds()
	draw.Draw(dst, b.Add(at.Add(p.at).Add(image.Pt(0, dy))), p.img, b.Min, draw.Over)
	return true
}

// drawAlpha draws a picture at `at` through a uniform alpha.
func drawAlpha(dst *image.RGBA, img *image.RGBA, at image.Point, alpha float64) {
	b := img.Bounds()
	if alpha >= 1 {
		draw.Draw(dst, b.Add(at), img, b.Min, draw.Over)
		return
	}
	mask := image.NewUniform(color.Alpha{uint8(max(0, min(1, alpha))*255 + 0.5)})
	draw.DrawMask(dst, b.Add(at), img, b.Min, mask, image.Point{}, draw.Over)
}

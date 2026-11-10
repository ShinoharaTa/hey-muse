//go:build !dot && !spot

package display

import (
	"errors"
	"image"
	"log/slog"
	"os"
	"path/filepath"
	"time"

	"github.com/HuskerMinion/techo5/echod/internal/feature/home"
	"github.com/HuskerMinion/techo5/echod/internal/lib/safe"
)

// The room behind the character, as the hour and the sky outside are: optional pictures in
// charaDir/rooms, one per time of day and weather, the same room each time with the light and the
// window changed.
//
//	rooms/dawn_clear.png     before and just after sunrise
//	rooms/morning_cloudy.png until ten
//	rooms/day_rain.png       until an hour before sunset
//	rooms/evening_clear.png  until 40 minutes after it
//	rooms/night_clear.png    the rest
//
// A missing picture falls back to the same hour's clear one, then to background.png. Only the room
// in use is kept, scaled to the screen: fifteen rooms decoded would be a hundred megabytes.
const charaRoomsDir = "rooms"

// Times of day, as the files are named.
const (
	roomDawn    = "dawn"
	roomMorning = "morning"
	roomDay     = "day"
	roomEvening = "evening"
	roomNight   = "night"
)

// Weather, as the files are named.
const (
	roomClear  = "clear"
	roomCloudy = "cloudy"
	roomRain   = "rain"
)

// roomHour is the time of day at now, from today's sunrise and sunset; ok false (home's place not
// known yet, or a day the sun does not rise or set) uses 6:00 and 17:30 in their place.
func roomHour(now, rise, set time.Time, ok bool) string {
	if !ok {
		day := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location())
		rise, set = day.Add(6*time.Hour), day.Add(17*time.Hour+30*time.Minute)
	}
	ten := time.Date(now.Year(), now.Month(), now.Day(), 10, 0, 0, 0, now.Location())
	switch {
	case now.Before(rise.Add(-time.Hour)):
		return roomNight
	case now.Before(rise.Add(30 * time.Minute)):
		return roomDawn
	case now.Before(ten) && now.Before(set.Add(-time.Hour)):
		return roomMorning
	case now.Before(set.Add(-time.Hour)):
		return roomDay
	case now.Before(set.Add(40 * time.Minute)):
		return roomEvening
	}
	return roomNight
}

// roomSky is the weather a Home Assistant condition shows through the window: fog and snow as an
// overcast sky, thunder as rain, a few clouds as clear. Nothing known is clear.
func roomSky(cond string) string {
	switch cond {
	case "cloudy", "fog", "snowy":
		return roomCloudy
	case "rainy", "pouring", "lightning", "lightning-rainy", "hail", "snowy-rainy":
		return roomRain
	}
	return roomClear
}

// charaRoomSun is today's sunrise and sunset, for roomHour; a variable so a test can set the hour.
var charaRoomSun = func(now time.Time) (rise, set time.Time, ok bool) { return home.Get().SunTimes(now) }

// charaRoom is the room in use, scaled to one screen size.
type charaRoom struct {
	name string // the file, "night_rain.png", or "" for background.png
	size image.Point
	img  *image.RGBA
	// tint is how the room's light colors what is drawn in it, by channel: its average color over
	// the clear day room's (1 in the day room, and with no day room to measure against).
	tint [3]float64
}

// roomFile is the picture to use for an hour and weather: the exact one, the hour's clear one, or ""
// for background.png.
func (c *charaSet) roomFile(hour, sky string) string {
	for _, name := range []string{hour + "_" + sky + ".png", hour + "_" + roomClear + ".png"} {
		if c.rooms[name] {
			return name
		}
	}
	return ""
}

// roomAt is the room for now and the weather, at w by h, or the room shown until now while that one is
// still being scaled (a second or so on the device, off the frame), or nil for background.png.
func (c *charaSet) roomAt(w, h int, now time.Time, cond string) *image.RGBA {
	if len(c.rooms) == 0 {
		return nil
	}
	rise, set, ok := charaRoomSun(now)
	name := c.roomFile(roomHour(now, rise, set, ok), roomSky(cond))
	size := image.Pt(w, h)
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.room.name == name && c.room.size == size {
		return c.room.img
	}
	if name != "" && c.roomPending != name {
		c.roomPending = name
		path := filepath.Join(c.dir, charaRoomsDir, name)
		safe.Go("chara room", func() {
			img, err := readPNG(path)
			var scaled *image.RGBA
			if err == nil {
				scaled = coverScale(img, w, h)
			} else {
				slog.Warn("character room not loaded", "file", path, "err", err)
			}
			tint := c.roomTint(scaled, w, h)
			c.mu.Lock()
			defer c.mu.Unlock()
			if c.roomPending == name {
				c.roomPending = ""
			}
			if scaled != nil {
				c.room = charaRoom{name: name, size: size, img: scaled, tint: tint}
				clear(c.lit)
			}
		})
	}
	if name == "" {
		c.room = charaRoom{size: size}
		return nil
	}
	if c.room.size != size {
		return nil
	}
	return c.room.img
}

// findRooms lists the room pictures in dir, by file name.
func findRooms(dir string) map[string]bool {
	entries, err := os.ReadDir(filepath.Join(dir, charaRoomsDir))
	if err != nil {
		if !errors.Is(err, os.ErrNotExist) {
			slog.Warn("character rooms not read", "dir", dir, "err", err)
		}
		return nil
	}
	rooms := map[string]bool{}
	for _, e := range entries {
		for _, hour := range []string{roomDawn, roomMorning, roomDay, roomEvening, roomNight} {
			for _, sky := range []string{roomClear, roomCloudy, roomRain} {
				if e.Name() == hour+"_"+sky+".png" {
					rooms[e.Name()] = true
				}
			}
		}
	}
	return rooms
}

// roomShown is the room picture in use, so the idle page draws again when a new one is ready.
func (c *charaSet) roomShown() string {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.room.name
}

// roomDayClear is the room the character is drawn for: the others tint it by how their light differs.
const roomDayClear = roomDay + "_" + roomClear + ".png"

// Tint bounds: the character takes three quarters of the room's change in light, and keeps at least
// half its brightness, so it stays readable in the darkest room.
const (
	tintShare = 0.75
	tintFloor = 0.5
)

// roomTint is the tint of a room scaled to w by h, measured against the clear day room (read once).
func (c *charaSet) roomTint(room *image.RGBA, w, h int) [3]float64 {
	none := [3]float64{1, 1, 1}
	if room == nil || !c.rooms[roomDayClear] {
		return none
	}
	c.mu.Lock()
	ref, ok := c.dayMean, c.dayMeasured
	c.mu.Unlock()
	if !ok {
		img, err := readPNG(filepath.Join(c.dir, charaRoomsDir, roomDayClear))
		if err != nil {
			return none
		}
		ref = meanRGB(coverScale(img, w, h))
		c.mu.Lock()
		c.dayMean, c.dayMeasured = ref, true
		c.mu.Unlock()
	}
	return tintFor(meanRGB(room), ref)
}

// tintFor is the tint of a room whose average color is m, the day room's ref.
func tintFor(m, ref [3]float64) [3]float64 {
	var t [3]float64
	for k := range t {
		r := 1.0
		if ref[k] > 0 {
			r = m[k] / ref[k]
		}
		t[k] = min(1, max(tintFloor, 1-tintShare*(1-r)))
	}
	return t
}

// meanRGB is a picture's average color, from every fourth pixel each way.
func meanRGB(img *image.RGBA) [3]float64 {
	var sum [3]float64
	n := 0
	b := img.Bounds()
	for y := b.Min.Y; y < b.Max.Y; y += 4 {
		for x := b.Min.X; x < b.Max.X; x += 4 {
			i := img.PixOffset(x, y)
			sum[0] += float64(img.Pix[i])
			sum[1] += float64(img.Pix[i+1])
			sum[2] += float64(img.Pix[i+2])
			n++
		}
	}
	if n == 0 {
		return sum
	}
	return [3]float64{sum[0] / float64(n), sum[1] / float64(n), sum[2] / float64(n)}
}

// tinted is a copy of a picture with its color channels scaled by t. The pixels are premultiplied,
// so scaling the color down alone keeps them valid.
func tinted(src *image.RGBA, t [3]float64) *image.RGBA {
	out := image.NewRGBA(src.Rect)
	copy(out.Pix, src.Pix)
	var lut [3][256]uint8
	for k := range lut {
		for v := range 256 {
			lut[k][v] = uint8(float64(v)*t[k] + 0.5)
		}
	}
	for i := 0; i+3 < len(out.Pix); i += 4 {
		out.Pix[i] = lut[0][out.Pix[i]]
		out.Pix[i+1] = lut[1][out.Pix[i+1]]
		out.Pix[i+2] = lut[2][out.Pix[i+2]]
	}
	return out
}

// litIn is a picture in the room's light, made once per room and kept until the room changes: key names
// the picture.
func (c *charaSet) litIn(key string, src *image.RGBA) *image.RGBA {
	c.mu.Lock()
	defer c.mu.Unlock()
	t := c.room.tint
	if c.room.img == nil || t == [3]float64{1, 1, 1} || t == [3]float64{} {
		return src
	}
	if img := c.lit[key]; img != nil && img.Rect == src.Rect {
		return img
	}
	if c.lit == nil {
		c.lit = map[string]*image.RGBA{}
	}
	img := tinted(src, t)
	c.lit[key] = img
	return img
}

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
			c.mu.Lock()
			defer c.mu.Unlock()
			if c.roomPending == name {
				c.roomPending = ""
			}
			if scaled != nil {
				c.room = charaRoom{name: name, size: size, img: scaled}
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

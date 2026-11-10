//go:build !dot && !spot

package display

import (
	"encoding/json"
	"log/slog"
	"os"
	"path/filepath"
	"sync"
)

// What the person chose for the character's idle page, kept beside the pictures so it survives a
// restart: which weather the tiles show. The file is the daemon's to write; chara.json stays the
// person's.

const charaViewFile = "view.json"

// The weather tiles' views, in the order a tap steps through them.
const (
	tileDays = "days" // today and tomorrow, side by side
	tileWeek = "week" // five days in one wide tile
	tileNow  = "now"  // the temperature now, large, with today's range
)

var tileViews = []string{tileDays, tileWeek, tileNow}

type charaView struct {
	Weather string `json:"weather,omitempty"`
	Layout  string `json:"layout,omitempty"`
}

// The idle page's layouts, in the order a swipe to the left steps through them.
const (
	layoutStandard = "standard" // the clock, two tiles and the buttons at the left, the character at the right
	layoutChara    = "chara"    // the character large near the middle, a small clock and the weather at the top left
	layoutClock    = "clock"    // a large clock and the date, the character small at the bottom right
)

var layouts = []string{layoutStandard, layoutChara, layoutClock}

// layoutNames are what a swipe says it turned to.
var layoutNames = map[string]string{layoutStandard: "いつもの", layoutChara: "キャラ大", layoutClock: "時計大"}

// layout is the page's layout, the standard one unless something else was chosen.
func (v charaView) layout() string {
	for _, l := range layouts {
		if v.Layout == l {
			return l
		}
	}
	return layoutStandard
}

// stepLayout turns the page to the next layout (+1) or the one before (-1), and returns it.
func stepLayout(step int) string {
	var to string
	charaViewSet(func(v *charaView) {
		cur, n := v.layout(), len(layouts)
		for i, l := range layouts {
			if l == cur {
				to = layouts[((i+step)%n+n)%n]
			}
		}
		v.Layout = to
	})
	return to
}

var (
	charaViewMu   sync.Mutex
	charaViewSeen bool
	charaViewNow  charaView
)

// charaViewGet is the saved choice, read from the file the first time it is asked for.
func charaViewGet() charaView {
	charaViewMu.Lock()
	defer charaViewMu.Unlock()
	if !charaViewSeen {
		charaViewSeen = true
		if b, err := os.ReadFile(filepath.Join(charaDir, charaViewFile)); err == nil {
			if err := json.Unmarshal(b, &charaViewNow); err != nil {
				slog.Warn("character view not read", "err", err)
			}
		}
	}
	return charaViewNow
}

// charaViewSet changes the choice and saves it.
func charaViewSet(change func(*charaView)) {
	v := charaViewGet()
	change(&v)
	charaViewMu.Lock()
	charaViewNow = v
	charaViewMu.Unlock()
	b, _ := json.Marshal(v)
	path := filepath.Join(charaDir, charaViewFile)
	if err := os.WriteFile(path+".tmp", b, 0o644); err == nil {
		err = os.Rename(path+".tmp", path)
		if err != nil {
			slog.Warn("character view not saved", "err", err)
		}
	} else {
		slog.Warn("character view not saved", "err", err)
	}
}

// weatherView is the tiles' view, today and tomorrow unless something else was chosen.
func (v charaView) weatherView() string {
	for _, w := range tileViews {
		if v.Weather == w {
			return w
		}
	}
	return tileDays
}

// nextWeatherView steps the tiles to the next view.
func nextWeatherView() {
	charaViewSet(func(v *charaView) {
		cur := v.weatherView()
		for i, w := range tileViews {
			if w == cur {
				v.Weather = tileViews[(i+1)%len(tileViews)]
				return
			}
		}
	})
}

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

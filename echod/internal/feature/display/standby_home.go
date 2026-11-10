//go:build !dot && !spot

package display

import (
	"context"
	"encoding/json"
	"github.com/HuskerMinion/techo5/echod/internal/feature/voice"
	"github.com/HuskerMinion/techo5/echod/internal/lib/mothership"
	"github.com/HuskerMinion/techo5/echod/internal/lib/standalone"
	"image"
	"os"
	"strings"
	"sync"
	"time"
)

type homeScene = standalone.Scene
type homeEvent = standalone.Event
type homeConfig struct {
	Key, URL string
	Scenes   []homeScene
}
type homeEnvironment = standalone.Environment

type standbyView struct {
	Scope                  string
	ScopeUntil             time.Time
	Environment            *homeEnvironment
	EnvironmentUpdated     time.Time
	EnvironmentUnavailable bool
	Enabled, Unavailable   bool
	Scenes                 []homeScene
	Events                 []homeEvent
	Updated                time.Time
	Page                   int
	Status                 string
}
type standbyHome struct {
	mu      sync.Mutex
	cfg     homeConfig
	view    standbyView
	ctx     context.Context
	busy    bool
	lastTap time.Time
}
type homeZone struct {
	bounds image.Rectangle
	action string
}
type homeResult = standalone.Result

// Replace just the power slots. A published scene slice may still be in use by
// the renderer, so never mutate it in place.
func (h *standbyHome) powerButtons(controls []homeScene) {
	if len(controls) == 0 {
		return
	}
	next := append([]homeScene(nil), h.view.Scenes...)
	for i, b := range next {
		for _, p := range controls {
			parts := strings.Split(p.ID, "/")
			if len(parts) == 3 && parts[0] == "power" && strings.HasPrefix(b.ID, "power/"+parts[1]+"/") {
				next[i] = p
				break
			}
		}
	}
	h.view.Scenes = next
}

func (h *standbyHome) request(ctx context.Context, operation, id string) (homeResult, error) {
	if standalone.Device.Enabled() {
		return standalone.Device.Dashboard(ctx, operation, id)
	}
	var out homeResult
	if operation == "scene" || operation == "power_status" {
		if err := mothership.Wait(ctx, h.cfg.URL); err != nil {
			return out, err
		}
	}
	err := mothership.HomeRequest(ctx, h.cfg.URL, h.cfg.Key, map[string]any{"op": operation, "id": id, "time": time.Now().Unix()}, &out)
	return out, err
}

func (h *standbyHome) run(ctx context.Context, wake func()) {
	raw, err := os.ReadFile("/data/misc/techo5/idle-home.json")
	var cfg homeConfig
	if err != nil || json.Unmarshal(raw, &cfg) != nil || (!standalone.Device.Enabled() && (len(cfg.Key) != 64 || cfg.URL == "")) {
		return
	}
	h.mu.Lock()
	h.cfg, h.ctx = cfg, ctx
	h.view = standbyView{Enabled: true, Scenes: cfg.Scenes}
	h.mu.Unlock()
	wake()
	go h.pollEnvironment(ctx, wake)
	for {
		h.mu.Lock()
		previousTap := h.lastTap
		h.mu.Unlock()
		resp, err := h.request(ctx, "agenda", "")
		h.mu.Lock()
		ok := err == nil && resp.OK
		h.view.Unavailable = !ok || resp.Stale
		if err == nil && !h.busy && h.lastTap == previousTap {
			h.powerButtons(resp.Controls)
		}
		if ok {
			h.view.Events = resp.Events
			h.view.Updated = resp.Fetched
			if h.view.Updated.IsZero() {
				h.view.Updated = time.Now()
			}
			h.view.Page = 0
		}
		h.mu.Unlock()
		wake()
		delay := 5 * time.Minute
		if !ok {
			delay = 30 * time.Second
		}
		select {
		case <-ctx.Done():
			return
		case <-time.After(delay):
		}
	}
}

func (h *standbyHome) pollEnvironment(ctx context.Context, wake func()) {
	for ctx.Err() == nil {
		result, err := h.request(ctx, "environment", "")
		ok := err == nil && result.OK && result.Environment != nil
		h.mu.Lock()
		h.view.EnvironmentUnavailable = !ok
		if ok {
			h.view.Environment = result.Environment
			h.view.EnvironmentUpdated = time.Now()
		}
		h.mu.Unlock()
		wake()
		select {
		case <-ctx.Done():
			return
		case <-time.After(5 * time.Minute):
		}
	}
}

func (h *standbyHome) snapshot() standbyView {
	h.mu.Lock()
	defer h.mu.Unlock()
	return h.view // published slices are immutable
}

func (d *Display) tapStandby(at image.Point) bool {
	d.r.zmu.Lock()
	action := ""
	for _, zone := range d.r.homeZones {
		if at.In(zone.bounds) {
			action = zone.action
			break
		}
	}
	d.r.zmu.Unlock()
	if action == "" {
		return false
	}
	switch action {
	case "weather":
		d.ShowWeather(false)
		return true
	case "weathertile":
		// The character's weather tiles: the next of today and tomorrow, the week, and now.
		nextWeatherView()
		d.wake()
		return true
	case "talk":
		// The character's Talk button: a turn, as a tap on the clock is.
		voice.Get().Action()
		return true
	case "pet":
		// A pat on the character's head: it hops and smiles (anim_chara.go).
		d.r.chara.pet(time.Now())
		d.wake()
		return true
	case "radio":
		d.openDrawer(drawerRadio)
		return true
	case "settings":
		if !d.OpenSheet("general") {
			d.showSheet(true)
		}
		return true
	}
	h := &d.standby
	h.mu.Lock()
	if action == "agenda" {
		h.view.Page++
		h.mu.Unlock()
		d.wake()
		return true
	}
	if h.busy || time.Since(h.lastTap) < 2*time.Second {
		h.mu.Unlock()
		return true
	}
	var label string
	for _, s := range h.view.Scenes {
		if s.ID == action {
			label = s.Label
			break
		}
	}
	if label == "" {
		h.mu.Unlock()
		return true
	}
	op := "scene"
	h.busy, h.lastTap = true, time.Now()
	h.view.Status = label + " を送信中"
	if strings.HasSuffix(action, "/setup") {
		op = "power_status"
		h.view.Status = "シーンを確認中"
	}
	ctx := h.ctx
	h.mu.Unlock()
	d.wake()
	go func() {
		result, err := h.request(ctx, op, action)
		h.mu.Lock()
		h.busy = false
		h.lastTap = time.Now()
		if err == nil {
			h.powerButtons(result.Controls)
		}
		if op == "power_status" && err == nil && result.OK {
			h.view.Status = "ボタンを更新しました"
			for _, p := range result.Controls {
				if p.ID == action {
					h.view.Status = "SwitchBotの手動シーンを設定してください"
				}
			}
		} else if err == nil && result.OK {
			h.view.Status = label + "：受付済み"
		} else {
			h.view.Status = label + "：実行を確認できません"
		}
		stamp := h.lastTap
		h.mu.Unlock()
		d.wake()
		select {
		case <-ctx.Done():
			return
		case <-time.After(7 * time.Second):
		}
		h.mu.Lock()
		if stamp.Equal(h.lastTap) {
			h.view.Status = ""
		}
		h.mu.Unlock()
		d.wake()
	}()
	return true
}

package home

import (
	"context"
	"fmt"
	"log/slog"
	"math"
	"time"

	"github.com/HuskerMinion/techo5/echod/internal/config"
	"github.com/HuskerMinion/techo5/echod/internal/lib/hass"
	"github.com/HuskerMinion/techo5/echod/internal/lib/openmeteo"
)

// The device's own weather, for a device with no Home Assistant: Open-Meteo at the place kept on the
// device (config.Home.Place). Home Assistant's weather entity wins whenever it has something to say, so
// a device that has one never sees this.

// ownDays covers today and the following six days.
const ownDays = 7

// ownHours is how far ahead the hourly forecast goes: a day, for "このあと" on the idle page.
const ownHours = 24

// ownStale is how old the device's own reading may be before it is not shown: a device that has lost
// its network should not go on showing the morning's sunshine at night.
const ownStale = 3 * time.Hour

func (f *Feature) refreshOwnWeather() {
	h := config.Get().Home
	if !h.Place.Set() {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	now, days, hours, err := openmeteo.ForecastHours(ctx, h.Place.Lat, h.Place.Lon, h.Fahrenheit(), ownDays, ownHours)
	if err != nil {
		slog.Warn("home: the device's own weather", "err", err)
		return
	}
	out := make([]hass.Day, len(days))
	for i, d := range days {
		out[i] = hass.Day{When: d.When, Condition: d.Condition, High: d.High, Low: d.Low, Rain: d.Rain}
	}
	f.mu.Lock()
	f.own, f.ownAt = now, time.Now()
	f.forecast, f.fetched = out, time.Now()
	f.hours = hours
	f.mu.Unlock()
	f.Changed.Emit(struct{}{})
}

func (f *Feature) ownWeather() Weather {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.ownAt.IsZero() || time.Since(f.ownAt) > ownStale || f.own.Condition == "" {
		return Weather{}
	}
	// Adding zero turns the -0 that rounding -0.4 gives into 0, which is what a thermometer says.
	return Weather{Condition: f.own.Condition, Temp: fmt.Sprintf("%.0f°", math.Round(f.own.Temp)+0)}
}

// Hours is the hourly forecast as last fetched, from the hour under way on; none with Home Assistant's
// weather, or while the device's own is stale.
func (f *Feature) Hours(now time.Time) []openmeteo.Hour {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.ownAt.IsZero() || time.Since(f.ownAt) > ownStale {
		return nil
	}
	start := now.Truncate(time.Hour)
	for i, h := range f.hours {
		if !h.When.Before(start) {
			return f.hours[i:]
		}
	}
	return nil
}

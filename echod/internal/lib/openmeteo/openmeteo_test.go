package openmeteo

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
)

// Every WMO code lands on a condition the screen can draw, and night is night.
func TestConditionNamesWhatTheScreenDraws(t *testing.T) {
	known := map[string]bool{"sunny": true, "clear-night": true, "partlycloudy": true, "cloudy": true, "rainy": true,
		"pouring": true, "lightning-rainy": true, "snowy": true, "snowy-rainy": true, "hail": true, "fog": true, "exceptional": true}
	for _, code := range []int{0, 1, 2, 3, 45, 48, 51, 53, 55, 56, 57, 61, 63, 65, 66, 67, 71, 73, 75, 77, 80, 81, 82, 85, 86, 95, 96, 99} {
		for _, day := range []bool{true, false} {
			if c := Condition(code, day); !known[c] || c == "exceptional" {
				t.Errorf("code %d day %v: %q", code, day, c)
			}
		}
	}
	if Condition(0, false) != "clear-night" || Condition(0, true) != "sunny" {
		t.Error("a clear night is not a clear day")
	}
}

// The forecast is read the way Open-Meteo sends it, a missing chance of rain as unknown.
func TestForecastReadsTheAnswer(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("temperature_unit") != "fahrenheit" {
			t.Errorf("asked in %q", r.URL.Query().Get("temperature_unit"))
		}
		_, _ = w.Write([]byte(`{"current":{"temperature_2m":44.8,"weather_code":51,"is_day":0},
		 "daily":{"time":["2026-09-29","2026-09-30"],"weather_code":[81,85],"temperature_2m_max":[54.1,40],
		 "temperature_2m_min":[41,35.7],"precipitation_probability_max":[41,null]}}`))
	}))
	defer srv.Close()
	old := forecastURL
	forecastURL = srv.URL
	defer func() { forecastURL = old }()

	now, days, err := Forecast(context.Background(), 61.2, -149.9, true, 2)
	if err != nil {
		t.Fatal(err)
	}
	if now.Condition != "rainy" || now.Temp != 44.8 {
		t.Errorf("now = %+v", now)
	}
	if len(days) != 2 || days[0].High != 54.1 || days[0].Rain != 41 || days[1].Condition != "snowy" || days[1].Rain != -1 {
		t.Errorf("days = %+v", days)
	}
}

// TestForecastHoursReadsTheHours asks for the hours only when wanted, and reads them with their night
// form after dark and no probability where none is given.
func TestForecastHoursReadsTheHours(t *testing.T) {
	var asked string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		asked = r.URL.Query().Get("hourly") + "|" + r.URL.Query().Get("forecast_hours")
		w.Write([]byte(`{"current":{"temperature_2m":11.9,"weather_code":3,"is_day":1},
			"daily":{"time":["2026-11-10"],"weather_code":[61],"temperature_2m_max":[13],"temperature_2m_min":[7],"precipitation_probability_max":[90]},
			"hourly":{"time":["2026-11-10T15:00","2026-11-10T18:00"],"temperature_2m":[12.0,11.1],"weather_code":[3,0],"precipitation_probability":[10,null],"is_day":[1,0]}}`))
	}))
	defer srv.Close()
	old := forecastURL
	forecastURL = srv.URL
	defer func() { forecastURL = old }()
	_, days, hours, err := ForecastHours(t.Context(), 35.9, 139.6, false, 1, 24)
	if err != nil {
		t.Fatal(err)
	}
	if asked != "temperature_2m,weather_code,precipitation_probability,is_day|24" || len(days) != 1 {
		t.Fatalf("asked %q, %d days", asked, len(days))
	}
	if len(hours) != 2 || hours[0].When.Hour() != 15 || hours[0].Rain != 10 || hours[0].Condition != "cloudy" ||
		hours[1].Condition != "clear-night" || hours[1].Rain != -1 || hours[1].Temp != 11.1 {
		t.Fatalf("hours %+v", hours)
	}
	if _, _, err := Forecast(t.Context(), 35.9, 139.6, false, 1); err != nil || asked != "|" {
		t.Fatalf("Forecast asked for hours: %q %v", asked, err)
	}
}

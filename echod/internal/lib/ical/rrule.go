package ical

import (
	"errors"
	"strings"
	"time"

	rrule "github.com/teambition/rrule-go"
)

// A recurrence set includes DTSTART and explicit extra dates even without an RRULE.
// Walk it once, in order, so duplicate dates and exclusions are applied together.
func (v *vevent) occurrences(from, to time.Time) ([]time.Time, error) {
	var set rrule.Set
	set.RDate(v.start)
	for _, t := range v.rdates {
		set.RDate(t)
	}
	for _, t := range v.exdates {
		set.ExDate(t)
	}
	if v.rrule != "" {
		opt, err := rrule.StrToROptionInLocation(strings.ToUpper(v.rrule), v.start.Location())
		if err == nil && opt.Interval >= 0 && opt.Count >= 0 {
			opt.Dtstart = v.start
			// A date-only UNTIL includes all occurrences on that day.
			for part := range strings.SplitSeq(v.rrule, ";") {
				if value, ok := strings.CutPrefix(strings.ToUpper(part), "UNTIL="); ok && len(value) == 8 && !opt.Until.IsZero() {
					opt.Until = opt.Until.AddDate(0, 0, 1).Add(-time.Second)
				}
			}
			if rule, err := rrule.NewRRule(*opt); err == nil {
				set.RRule(rule)
			}
		}
	}
	next := set.Iterator()
	var out []time.Time
	for i := 0; i < 1000000; i++ {
		t, ok := next()
		if !ok || !t.Before(to) {
			return out, nil
		}
		if !v.endFor(t).Before(from) {
			out = append(out, t)
		}
	}
	return nil, errors.New("calendar recurrence expansion exceeded its limit")
}

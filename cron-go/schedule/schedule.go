// Package schedule parses cron expressions and computes when they fire.
//
// Supported forms:
//
//	minute hour day-of-month month day-of-week   (5 fields)
//	@yearly @annually @monthly @weekly @daily @midnight @hourly
//	@every <duration>                            (e.g. @every 10m, at least 1s)
//
// Fields accept *, numbers, ranges (1-5), steps (*/15, 0-30/10, 5/10) and
// lists (1,15,30). Months and weekdays also accept names (jan, mon); a
// weekday of 7 is Sunday. As in Vixie cron, when both day-of-month and
// day-of-week are restricted, a day matching either one fires.
package schedule

import (
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"
)

// Schedule is a parsed expression.
type Schedule interface {
	// Next returns the first time strictly after t that the schedule fires,
	// in loc, or the zero time if it never does.
	Next(t time.Time, loc *time.Location) time.Time
}

// Parse parses an expression.
func Parse(expr string) (Schedule, error) {
	expr = strings.TrimSpace(expr)
	if rest, ok := strings.CutPrefix(expr, "@every "); ok {
		d, err := time.ParseDuration(strings.TrimSpace(rest))
		if err != nil {
			return nil, fmt.Errorf("@every: %w", err)
		}
		if d < time.Second {
			return nil, errors.New("@every: the interval must be at least 1s")
		}
		return every(d), nil
	}
	if macro, ok := macros[expr]; ok {
		expr = macro
	}
	fields := strings.Fields(expr)
	if len(fields) != 5 {
		return nil, fmt.Errorf("expected 5 fields (minute hour day month weekday), got %d", len(fields))
	}
	var c cron
	var err error
	for i, f := range []struct {
		dst      *uint64
		min, max int
		names    map[string]int
		label    string
	}{
		{&c.minute, 0, 59, nil, "minute"},
		{&c.hour, 0, 23, nil, "hour"},
		{&c.dom, 1, 31, nil, "day of month"},
		{&c.month, 1, 12, monthNames, "month"},
		{&c.dow, 0, 7, dayNames, "day of week"},
	} {
		if *f.dst, err = parseField(fields[i], f.min, f.max, f.names); err != nil {
			return nil, fmt.Errorf("%s: %w", f.label, err)
		}
	}
	if c.dow&(1<<7) != 0 { // 7 is Sunday too
		c.dow = c.dow&^(1<<7) | 1
	}
	c.domStar, c.dowStar = fields[2] == "*" || fields[2] == "?", fields[4] == "*" || fields[4] == "?"
	return c, nil
}

var macros = map[string]string{
	"@yearly": "0 0 1 1 *", "@annually": "0 0 1 1 *", "@monthly": "0 0 1 * *",
	"@weekly": "0 0 * * 0", "@daily": "0 0 * * *", "@midnight": "0 0 * * *", "@hourly": "0 * * * *",
}

var monthNames = map[string]int{"jan": 1, "feb": 2, "mar": 3, "apr": 4, "may": 5, "jun": 6, "jul": 7, "aug": 8, "sep": 9, "oct": 10, "nov": 11, "dec": 12}

var dayNames = map[string]int{"sun": 0, "mon": 1, "tue": 2, "wed": 3, "thu": 4, "fri": 5, "sat": 6}

// parseField returns the field's values as a bitset.
func parseField(field string, min, max int, names map[string]int) (uint64, error) {
	var bits uint64
	for _, part := range strings.Split(field, ",") {
		rangePart, stepText, hasStep := strings.Cut(part, "/")
		step := 1
		if hasStep {
			n, err := strconv.Atoi(stepText)
			if err != nil || n < 1 {
				return 0, fmt.Errorf("invalid step %q", stepText)
			}
			step = n
		}
		var lo, hi int
		switch {
		case rangePart == "*" || rangePart == "?":
			lo, hi = min, max
		case strings.Contains(rangePart, "-"):
			a, b, _ := strings.Cut(rangePart, "-")
			var err error
			if lo, err = value(a, min, max, names); err != nil {
				return 0, err
			}
			if hi, err = value(b, min, max, names); err != nil {
				return 0, err
			}
			if lo > hi {
				return 0, fmt.Errorf("range %q is backwards", rangePart)
			}
		default:
			n, err := value(rangePart, min, max, names)
			if err != nil {
				return 0, err
			}
			lo, hi = n, n
			if hasStep { // "5/10" means from 5 to the end, every 10
				hi = max
			}
		}
		for v := lo; v <= hi; v += step {
			bits |= 1 << v
		}
	}
	return bits, nil
}

func value(s string, min, max int, names map[string]int) (int, error) {
	if n, ok := names[strings.ToLower(s)]; ok {
		return n, nil
	}
	n, err := strconv.Atoi(s)
	if err != nil {
		return 0, fmt.Errorf("invalid value %q", s)
	}
	if n < min || n > max {
		return 0, fmt.Errorf("%d is out of range %d-%d", n, min, max)
	}
	return n, nil
}

type cron struct {
	minute, hour, dom, month, dow uint64
	domStar, dowStar              bool
}

func (c cron) dayMatches(t time.Time) bool {
	dom := c.dom&(1<<t.Day()) != 0
	dow := c.dow&(1<<int(t.Weekday())) != 0
	if c.domStar || c.dowStar {
		return dom && dow
	}
	return dom || dow
}

// Next walks forward field by field, skipping whole months, days and hours
// that cannot match, so it needs few steps even for sparse schedules.
func (c cron) Next(after time.Time, loc *time.Location) time.Time {
	if loc == nil {
		loc = time.Local
	}
	t := after.In(loc).Truncate(time.Minute).Add(time.Minute)
	limit := t.AddDate(5, 0, 0) // e.g. "0 0 30 2 *" never fires
	for t.Before(limit) {
		if c.month&(1<<int(t.Month())) == 0 {
			t = time.Date(t.Year(), t.Month()+1, 1, 0, 0, 0, 0, loc)
			continue
		}
		if !c.dayMatches(t) {
			t = time.Date(t.Year(), t.Month(), t.Day()+1, 0, 0, 0, 0, loc)
			continue
		}
		if c.hour&(1<<t.Hour()) == 0 {
			next := time.Date(t.Year(), t.Month(), t.Day(), t.Hour()+1, 0, 0, 0, loc)
			if !next.After(t) { // a DST transition repeated the hour
				next = t.Add(time.Hour).Truncate(time.Hour)
			}
			t = next
			continue
		}
		if c.minute&(1<<t.Minute()) == 0 {
			t = t.Add(time.Minute)
			continue
		}
		return t
	}
	return time.Time{}
}

type every time.Duration

func (e every) Next(after time.Time, loc *time.Location) time.Time {
	d := time.Duration(e)
	return after.Truncate(time.Second).Add(d)
}

// NextN returns up to n upcoming times after t, for previews.
func NextN(s Schedule, t time.Time, loc *time.Location, n int) []time.Time {
	var out []time.Time
	for len(out) < n {
		t = s.Next(t, loc)
		if t.IsZero() {
			break
		}
		out = append(out, t)
	}
	return out
}

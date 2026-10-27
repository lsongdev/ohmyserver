package schedule

import (
	"testing"
	"time"
)

func TestNext(t *testing.T) {
	shanghai, _ := time.LoadLocation("Asia/Shanghai")
	from := time.Date(2026, 10, 25, 10, 7, 30, 0, shanghai) // a Sunday
	cases := []struct {
		expr string
		want []string
	}{
		{"*/15 * * * *", []string{"10:15", "10:30", "10:45"}},
		{"0 9-18/3 * * *", []string{"12:00", "15:00", "18:00"}},
		{"5/20 10 * * *", []string{"10:25", "10:45", "2026-10-26 10:05"}},
		{"30 8 * * mon-fri", []string{"2026-10-26 08:30", "2026-10-27 08:30"}},
		{"0 0 1 jan,jul *", []string{"2027-01-01 00:00", "2027-07-01 00:00"}},
		{"@daily", []string{"2026-10-26 00:00"}},
		{"@hourly", []string{"11:00", "12:00"}},
		{"0 12 * * 7", []string{"12:00", "2026-11-01 12:00"}},
		// Both day fields restricted: either matches (the 1st, or Fridays).
		{"0 0 1 * fri", []string{"2026-10-30 00:00", "2026-11-01 00:00", "2026-11-06 00:00"}},
		{"0 0 29 2 *", []string{"2028-02-29 00:00"}},
	}
	for _, c := range cases {
		s, err := Parse(c.expr)
		if err != nil {
			t.Errorf("%s: %v", c.expr, err)
			continue
		}
		got := NextN(s, from, shanghai, len(c.want))
		for i, w := range c.want {
			layout := "15:04"
			if len(w) > 5 {
				layout = "2006-01-02 15:04"
			}
			if i >= len(got) || got[i].Format(layout) != w {
				t.Errorf("%s: run %d = %v, want %s", c.expr, i, got, w)
				break
			}
		}
	}
	if s, _ := Parse("0 0 30 2 *"); !s.Next(from, shanghai).IsZero() {
		t.Error("February 30th fired")
	}
	s, _ := Parse("@every 90s")
	if got := s.Next(from, shanghai); got.Sub(from) != 90*time.Second {
		t.Errorf("@every: %v", got)
	}
}

func TestTimezone(t *testing.T) {
	s, _ := Parse("0 9 * * *")
	utc := time.Date(2026, 10, 25, 0, 0, 0, 0, time.UTC)
	tokyo, _ := time.LoadLocation("Asia/Tokyo")
	// 00:00 UTC is 09:00 in Tokyo, and Next is strictly after: the next
	// day's 09:00.
	if got := s.Next(utc, tokyo); !got.Equal(time.Date(2026, 10, 26, 0, 0, 0, 0, time.UTC)) {
		t.Errorf("tokyo 09:00 = %v", got.UTC())
	}
	// Spring forward: 02:30 does not exist on 2026-03-08 in New York.
	ny, _ := time.LoadLocation("America/New_York")
	s, _ = Parse("30 * * * *")
	got := NextN(s, time.Date(2026, 3, 8, 0, 45, 0, 0, ny), ny, 3)
	for i := 1; i < len(got); i++ {
		if !got[i].After(got[i-1]) {
			t.Fatalf("not increasing across DST: %v", got)
		}
	}
}

func TestParseErrors(t *testing.T) {
	for _, bad := range []string{"", "* * * *", "60 * * * *", "* 24 * * *", "* * 0 * *", "* * * 13 *", "* * * * 8",
		"*/0 * * * *", "5-1 * * * *", "x * * * *", "@every 100ms", "@every soon", "@often"} {
		if _, err := Parse(bad); err == nil {
			t.Errorf("accepted %q", bad)
		}
	}
}

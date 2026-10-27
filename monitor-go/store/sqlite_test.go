package store

import (
	"context"
	"database/sql"
	"path/filepath"
	"testing"
	"time"

	"github.com/lsongdev/ohmyserver/monitor-go/metrics"
)

func open(t *testing.T) *SQLite {
	t.Helper()
	db, err := sql.Open("sqlite", filepath.Join(t.TempDir(), "panel.db")+"?_pragma=busy_timeout(5000)")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	s, err := New(context.Background(), db)
	if err != nil {
		t.Fatal(err)
	}
	return s
}

func count(t *testing.T, s *SQLite, res int) int {
	var n int
	s.db.QueryRow(`SELECT COUNT(*) FROM monitor_samples WHERE resolution = ?`, res).Scan(&n)
	return n
}

func TestCompactAndSeries(t *testing.T) {
	ctx := context.Background()
	s := open(t)
	now := time.Now().Truncate(time.Hour).Add(30 * time.Minute)
	// Three hours of samples every 15 s; CPU equals the minute of the hour.
	for ts := now.Add(-3 * time.Hour); ts.Before(now); ts = ts.Add(15 * time.Second) {
		if err := s.Add(ctx, "local", metrics.Sample{Time: ts, CPU: float64(ts.Minute()), MemTotal: 100, MemUsed: uint64(ts.Second())}); err != nil {
			t.Fatal(err)
		}
	}
	if err := s.Compact(ctx, now); err != nil {
		t.Fatal(err)
	}
	if n := count(t, s, Minute); n != 180 {
		t.Fatalf("minutes = %d, want 180", n)
	}
	if n := count(t, s, Hour); n != 3 { // the current hour is still filling
		t.Fatalf("hours = %d, want 3", n)
	}
	// Compacting again changes nothing.
	s.Compact(ctx, now)
	if count(t, s, Minute) != 180 {
		t.Fatal("not idempotent")
	}
	var avg float64
	var mem int64
	s.db.QueryRow(`SELECT cpu, mem_used FROM monitor_samples WHERE resolution = 1 AND ts = ?`, now.Add(-time.Hour).Truncate(time.Minute).Unix()).Scan(&avg, &mem)
	if avg != float64(now.Add(-time.Hour).Minute()) || mem != 22 { // seconds 0,15,30,45 average 22.5
		t.Fatalf("minute average cpu %v mem %v", avg, mem)
	}

	// One hour from raw samples, averaged down to about 60 points.
	series, step, err := s.Series(ctx, "local", now.Add(-time.Hour), now, 60)
	if err != nil || step != time.Minute || len(series) < 59 || len(series) > 61 {
		t.Fatalf("series %d points, step %v, %v", len(series), step, err)
	}
	// A day reads minutes.
	series, step, _ = s.Series(ctx, "local", now.Add(-24*time.Hour), now, 300)
	if step != 288*time.Second || len(series) == 0 {
		t.Fatalf("day: %d points, step %v", len(series), step)
	}

	// Retention: two days later raw samples are gone and averages remain;
	// the hour that was filling is complete now and averaged too.
	s.Compact(ctx, now.Add(48*time.Hour))
	if count(t, s, Raw) != 0 || count(t, s, Minute) != 180 || count(t, s, Hour) != 4 {
		t.Fatalf("after retention: raw %d minute %d hour %d", count(t, s, Raw), count(t, s, Minute), count(t, s, Hour))
	}
}

func TestHosts(t *testing.T) {
	ctx := context.Background()
	s := open(t)
	s.AddHost(ctx, "local", true)
	s.AddHost(ctx, "ssh:1", true)
	s.SetEnabled(ctx, "ssh:1", false)
	s.AddHost(ctx, "ssh:1", true) // already known: unchanged
	now := time.Now().Truncate(time.Second)
	s.Seen(ctx, "local", metrics.Info{Hostname: "mac"}, now)
	hosts, err := s.Hosts(ctx)
	if err != nil || len(hosts) != 2 || hosts["ssh:1"].Enabled || hosts["local"].Info.Hostname != "mac" || !hosts["local"].LastSeen.Equal(now) {
		t.Fatalf("hosts %+v %v", hosts, err)
	}
	s.Add(ctx, "ssh:1", metrics.Sample{Time: now})
	s.DeleteHost(ctx, "ssh:1")
	if hosts, _ := s.Hosts(ctx); len(hosts) != 1 || count(t, s, Raw) != 0 {
		t.Fatal("delete")
	}
}

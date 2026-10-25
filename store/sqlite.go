// Package store keeps hosts and their samples in SQLite, at three
// resolutions: raw samples for a day, one-minute averages for a week and
// one-hour averages for three months, so the database stays small however
// long it runs.
package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/lsongdev/monitor-go/metrics"
	_ "modernc.org/sqlite" // registers the "sqlite" driver for Open
)

// Resolutions of stored samples, and how long each is kept.
const (
	Raw    = 0
	Minute = 1
	Hour   = 2
)

var (
	bucket    = map[int]int64{Raw: 0, Minute: 60, Hour: 3600}
	retention = map[int]time.Duration{Raw: 24 * time.Hour, Minute: 7 * 24 * time.Hour, Hour: 90 * 24 * time.Hour}
)

// Host is a monitored host's stored state.
type Host struct {
	ID       string       `json:"id"`
	Enabled  bool         `json:"enabled"`
	Info     metrics.Info `json:"info"`
	LastSeen time.Time    `json:"last_seen,omitzero"`
}

// SQLite stores hosts and samples. Its tables are prefixed "monitor_", so
// the database can be shared with other modules.
type SQLite struct {
	db    *sql.DB
	owned bool // opened by Open, so Close closes it
}

// Open opens (creating if needed) a database file of its own at path.
func Open(ctx context.Context, path string) (*SQLite, error) {
	db, err := sql.Open("sqlite", path+"?_pragma=busy_timeout(5000)&_pragma=journal_mode(WAL)&_txlock=immediate")
	if err != nil {
		return nil, err
	}
	db.SetMaxOpenConns(1)
	s, err := New(ctx, db)
	if err != nil {
		db.Close()
		return nil, err
	}
	s.owned = true
	return s, nil
}

// New migrates the monitor_ tables in db, which the caller owns and may
// share with other modules.
func New(ctx context.Context, db *sql.DB) (*SQLite, error) {
	s := &SQLite{db: db}
	if err := s.migrate(ctx); err != nil {
		return nil, fmt.Errorf("migrate database: %w", err)
	}
	return s, nil
}

// Close closes the database if Open opened it.
func (s *SQLite) Close() error {
	if s.owned {
		return s.db.Close()
	}
	return nil
}

// migrations are applied in order; monitor_schema records how many have
// run. Append new migrations, never edit old ones.
var migrations = []string{`
CREATE TABLE monitor_hosts (
	id        TEXT PRIMARY KEY,           -- collector host ID, e.g. "local", "ssh:3"
	enabled   INTEGER NOT NULL,
	info      TEXT NOT NULL DEFAULT '{}', -- metrics.Info as JSON
	last_seen INTEGER NOT NULL DEFAULT 0  -- unix seconds of the last reading
);
CREATE TABLE monitor_samples (
	host       TEXT NOT NULL,
	resolution INTEGER NOT NULL,          -- 0 raw, 1 minute, 2 hour
	ts         INTEGER NOT NULL,          -- unix seconds; bucket start for averages
	cpu        REAL NOT NULL,
	load1      REAL NOT NULL,
	load5      REAL NOT NULL,
	load15     REAL NOT NULL,
	mem_total  INTEGER NOT NULL,
	mem_used   INTEGER NOT NULL,
	swap_total INTEGER NOT NULL,
	swap_used  INTEGER NOT NULL,
	disk_total INTEGER NOT NULL,
	disk_used  INTEGER NOT NULL,
	disk_read  REAL NOT NULL,             -- bytes per second
	disk_write REAL NOT NULL,
	net_recv   REAL NOT NULL,
	net_sent   REAL NOT NULL,
	PRIMARY KEY (host, resolution, ts)
) WITHOUT ROWID`}

func (s *SQLite) migrate(ctx context.Context) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err := tx.ExecContext(ctx, `CREATE TABLE IF NOT EXISTS monitor_schema (version INTEGER NOT NULL)`); err != nil {
		return err
	}
	var version int
	if err := tx.QueryRowContext(ctx, `SELECT COALESCE(MAX(version), 0) FROM monitor_schema`).Scan(&version); err != nil {
		return err
	}
	if version > len(migrations) {
		return fmt.Errorf("schema version %d is newer than this program (%d)", version, len(migrations))
	}
	if version == len(migrations) {
		return nil
	}
	for i := version; i < len(migrations); i++ {
		if _, err := tx.ExecContext(ctx, migrations[i]); err != nil {
			return fmt.Errorf("migration %d: %w", i+1, err)
		}
	}
	if _, err := tx.ExecContext(ctx, `DELETE FROM monitor_schema`); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, `INSERT INTO monitor_schema (version) VALUES (?)`, len(migrations)); err != nil {
		return err
	}
	return tx.Commit()
}

// Hosts returns every known host.
func (s *SQLite) Hosts(ctx context.Context) (map[string]Host, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT id, enabled, info, last_seen FROM monitor_hosts`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[string]Host{}
	for rows.Next() {
		var h Host
		var info string
		var seen int64
		if err := rows.Scan(&h.ID, &h.Enabled, &info, &seen); err != nil {
			return nil, err
		}
		json.Unmarshal([]byte(info), &h.Info)
		if seen > 0 {
			h.LastSeen = time.Unix(seen, 0)
		}
		out[h.ID] = h
	}
	return out, rows.Err()
}

// AddHost records a newly seen host; an existing one is left as is.
func (s *SQLite) AddHost(ctx context.Context, id string, enabled bool) error {
	_, err := s.db.ExecContext(ctx, `INSERT INTO monitor_hosts (id, enabled) VALUES (?, ?) ON CONFLICT (id) DO NOTHING`, id, enabled)
	return err
}

// SetEnabled turns monitoring of a host on or off.
func (s *SQLite) SetEnabled(ctx context.Context, id string, enabled bool) error {
	_, err := s.db.ExecContext(ctx, `UPDATE monitor_hosts SET enabled = ? WHERE id = ?`, enabled, id)
	return err
}

// DeleteHost removes a host and its history.
func (s *SQLite) DeleteHost(ctx context.Context, id string) error {
	if _, err := s.db.ExecContext(ctx, `DELETE FROM monitor_samples WHERE host = ?`, id); err != nil {
		return err
	}
	_, err := s.db.ExecContext(ctx, `DELETE FROM monitor_hosts WHERE id = ?`, id)
	return err
}

// Seen records a reading's host info.
func (s *SQLite) Seen(ctx context.Context, id string, info metrics.Info, at time.Time) error {
	raw, err := json.Marshal(info)
	if err != nil {
		return err
	}
	_, err = s.db.ExecContext(ctx, `UPDATE monitor_hosts SET info = ?, last_seen = ? WHERE id = ?`, string(raw), at.Unix(), id)
	return err
}

const sampleColumns = `cpu, load1, load5, load15, mem_total, mem_used, swap_total, swap_used, disk_total, disk_used, disk_read, disk_write, net_recv, net_sent`

// Add stores a raw sample.
func (s *SQLite) Add(ctx context.Context, host string, x metrics.Sample) error {
	_, err := s.db.ExecContext(ctx, `INSERT OR REPLACE INTO monitor_samples (host, resolution, ts, `+sampleColumns+`)
		VALUES (?, 0, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		host, x.Time.Unix(), x.CPU, x.Load1, x.Load5, x.Load15, x.MemTotal, x.MemUsed, x.SwapTotal, x.SwapUsed,
		x.DiskTotal, x.DiskUsed, x.DiskRead, x.DiskWrite, x.NetRecv, x.NetSent)
	return err
}

// averages averages every sample column over groups; integers stay
// integers.
var averages = func() string {
	var parts []string
	for _, c := range strings.Split(sampleColumns, ", ") {
		if strings.HasSuffix(c, "_total") || strings.HasSuffix(c, "_used") {
			parts = append(parts, "CAST(AVG("+c+") AS INTEGER)")
		} else {
			parts = append(parts, "AVG("+c+")")
		}
	}
	return strings.Join(parts, ", ")
}()

// Compact averages raw samples into minutes and minutes into hours for
// every complete bucket since the last day (rewriting buckets is
// idempotent), then drops samples past their retention.
func (s *SQLite) Compact(ctx context.Context, now time.Time) error {
	for _, step := range []struct{ from, to int }{{Raw, Minute}, {Minute, Hour}} {
		size := bucket[step.to]
		current := now.Unix() / size * size // the bucket still filling
		since := now.Add(-2*retention[Raw]).Unix() / size * size
		_, err := s.db.ExecContext(ctx, `INSERT OR REPLACE INTO monitor_samples (host, resolution, ts, `+sampleColumns+`)
			SELECT host, ?, ts / ? * ?, `+averages+` FROM monitor_samples
			WHERE resolution = ? AND ts >= ? AND ts < ? GROUP BY host, ts / ?`,
			step.to, size, size, step.from, since, current, size)
		if err != nil {
			return err
		}
	}
	for res, keep := range retention {
		if _, err := s.db.ExecContext(ctx, `DELETE FROM monitor_samples WHERE resolution = ? AND ts < ?`, res, now.Add(-keep).Unix()); err != nil {
			return err
		}
	}
	return nil
}

// Series returns a host's samples from from to to, at most about points of
// them: it reads the finest resolution that still covers from, and averages
// consecutive samples when there are more than points.
func (s *SQLite) Series(ctx context.Context, host string, from, to time.Time, points int) ([]metrics.Sample, time.Duration, error) {
	res := Raw
	switch age := time.Since(from); {
	case age > retention[Minute]:
		res = Hour
	case age > retention[Raw] || to.Sub(from) > 6*time.Hour:
		res = Minute
	}
	span := to.Unix() - from.Unix()
	step := max(bucket[res], span/int64(max(points, 1)))
	if step < 1 {
		step = 1
	}
	rows, err := s.db.QueryContext(ctx, `SELECT ts / ? * ?, `+averages+` FROM monitor_samples
		WHERE host = ? AND resolution = ? AND ts >= ? AND ts <= ? GROUP BY ts / ? ORDER BY 1`,
		step, step, host, res, from.Unix(), to.Unix(), step)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()
	out := []metrics.Sample{}
	for rows.Next() {
		var x metrics.Sample
		var ts int64
		if err := rows.Scan(&ts, &x.CPU, &x.Load1, &x.Load5, &x.Load15, &x.MemTotal, &x.MemUsed, &x.SwapTotal, &x.SwapUsed,
			&x.DiskTotal, &x.DiskUsed, &x.DiskRead, &x.DiskWrite, &x.NetRecv, &x.NetSent); err != nil {
			return nil, 0, err
		}
		x.Time = time.Unix(ts, 0)
		out = append(out, x)
	}
	return out, time.Duration(step) * time.Second, rows.Err()
}

// ErrNotFound reports an unknown host.
var ErrNotFound = errors.New("host not found")

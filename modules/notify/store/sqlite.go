// Package store keeps events and issues in SQLite, in tables prefixed
// "notify_", so the database can be shared with other modules.
package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"

	_ "modernc.org/sqlite" // registers the "sqlite" driver for Open
)

// ErrNotFound reports an issue that does not exist.
var ErrNotFound = errors.New("not found")

// Levels, least severe first.
const (
	LevelDebug    = "debug"
	LevelInfo     = "info"
	LevelWarning  = "warning"
	LevelCritical = "critical"
)

// Rank orders levels; an unknown one is 0.
func Rank(level string) int {
	switch level {
	case LevelInfo:
		return 1
	case LevelWarning:
		return 2
	case LevelCritical:
		return 3
	}
	return 0
}

// What an event says about its issue.
const (
	KindMessage   = "message"
	KindOpened    = "opened"
	KindEscalated = "escalated"
	KindResolved  = "resolved"
)

// Issue states.
const (
	StateOpen     = "open"
	StateResolved = "resolved"
)

// Event is one thing that happened. Events are never changed.
type Event struct {
	ID      int64     `json:"id"`
	Created time.Time `json:"created_at"`
	Source  string    `json:"source"`
	Level   string    `json:"level"`
	Kind    string    `json:"kind"`
	Title   string    `json:"title"`
	Body    string    `json:"body,omitempty"`
	Link    string    `json:"link,omitempty"`
	IssueID int64     `json:"issue_id,omitempty"`
}

// Issue is an ongoing problem: the events of one source that share a key.
type Issue struct {
	ID          int64      `json:"id"`
	Source      string     `json:"source"`
	Key         string     `json:"key"`
	State       string     `json:"state"`
	Level       string     `json:"level"`
	Title       string     `json:"title"`
	Body        string     `json:"body,omitempty"`
	Link        string     `json:"link,omitempty"`
	FirstSeen   time.Time  `json:"first_seen"`
	LastSeen    time.Time  `json:"last_seen"`
	ResolvedAt  *time.Time `json:"resolved_at,omitempty"`
	Occurrences int64      `json:"occurrences"`
	MutedUntil  *time.Time `json:"muted_until,omitempty"`
}

// SQLite stores events and issues.
type SQLite struct {
	db    *sql.DB
	owned bool // opened by Open, so Close closes it
}

// Open opens (creating if needed) a database file of its own at path.
func Open(ctx context.Context, path string) (*SQLite, error) {
	db, err := sql.Open("sqlite", path+"?_pragma=foreign_keys(1)&_pragma=busy_timeout(5000)&_pragma=journal_mode(WAL)&_txlock=immediate")
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

// New migrates the notify_ tables in db, which the caller owns and may share
// with other modules; db should enable foreign keys.
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

// migrations are applied in order; notify_schema records how many have run.
// Append new migrations, never edit old ones.
var migrations = []string{`
CREATE TABLE notify_issues (
	id              INTEGER PRIMARY KEY,
	source          TEXT NOT NULL,
	key             TEXT NOT NULL,
	state           TEXT NOT NULL,
	level           TEXT NOT NULL,
	title           TEXT NOT NULL,
	body            TEXT NOT NULL DEFAULT '',
	link            TEXT NOT NULL DEFAULT '',
	first_seen      INTEGER NOT NULL,
	last_seen       INTEGER NOT NULL,
	resolved_at     INTEGER,
	resolve_pending INTEGER NOT NULL DEFAULT 0,
	occurrences     INTEGER NOT NULL,
	UNIQUE (source, key)
);
CREATE INDEX notify_issues_state ON notify_issues(state);
CREATE TABLE notify_events (
	id         INTEGER PRIMARY KEY,
	created_at INTEGER NOT NULL,
	source     TEXT NOT NULL,
	level      TEXT NOT NULL,
	kind       TEXT NOT NULL,
	title      TEXT NOT NULL,
	body       TEXT NOT NULL DEFAULT '',
	link       TEXT NOT NULL DEFAULT '',
	issue_id   INTEGER REFERENCES notify_issues(id) ON DELETE SET NULL
);
CREATE INDEX notify_events_time ON notify_events(created_at);
CREATE INDEX notify_events_issue ON notify_events(issue_id);
`, `
ALTER TABLE notify_issues ADD COLUMN muted_until INTEGER;
CREATE TABLE notify_channels (
	id              INTEGER PRIMARY KEY,
	name            TEXT NOT NULL UNIQUE,
	type            TEXT NOT NULL,
	config          TEXT NOT NULL,
	min_level       TEXT NOT NULL,
	sources         TEXT NOT NULL DEFAULT '',
	enabled         INTEGER NOT NULL,
	last_error      TEXT NOT NULL DEFAULT '',
	last_error_at   INTEGER,
	last_success_at INTEGER,
	created_at      INTEGER NOT NULL
);
CREATE TABLE notify_deliveries (
	id              INTEGER PRIMARY KEY,
	event_id        INTEGER NOT NULL REFERENCES notify_events(id) ON DELETE CASCADE,
	channel_id      INTEGER NOT NULL REFERENCES notify_channels(id) ON DELETE CASCADE,
	state           TEXT NOT NULL,
	attempts        INTEGER NOT NULL DEFAULT 0,
	next_attempt_at INTEGER NOT NULL,
	finished_at     INTEGER,
	error           TEXT NOT NULL DEFAULT '',
	created_at      INTEGER NOT NULL
);
CREATE INDEX notify_deliveries_due ON notify_deliveries(channel_id, state, next_attempt_at);
CREATE INDEX notify_deliveries_event ON notify_deliveries(event_id);
`}

func (s *SQLite) migrate(ctx context.Context) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err := tx.ExecContext(ctx, `CREATE TABLE IF NOT EXISTS notify_schema (version INTEGER NOT NULL)`); err != nil {
		return err
	}
	var version int
	if err := tx.QueryRowContext(ctx, `SELECT COALESCE(MAX(version), 0) FROM notify_schema`).Scan(&version); err != nil {
		return err
	}
	for ; version < len(migrations); version++ {
		if _, err := tx.ExecContext(ctx, migrations[version]); err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx, `INSERT INTO notify_schema (version) VALUES (?)`, version+1); err != nil {
			return err
		}
	}
	return tx.Commit()
}

// Report is what a source says, as Record takes it.
type Report struct {
	Source, Key, Level, Title, Body, Link string
	Resolved                              bool
}

// emit writes an event dated at, and queues it for the channels it goes to.
// Nothing goes out for a muted issue, though it is still recorded.
func emit(ctx context.Context, tx *sql.Tx, at, now time.Time, r Report, kind string, issue int64, muted bool) error {
	var issueID any
	if issue != 0 {
		issueID = issue
	}
	res, err := tx.ExecContext(ctx, `INSERT INTO notify_events (created_at, source, level, kind, title, body, link, issue_id)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?)`, at.UnixMilli(), r.Source, r.Level, kind, r.Title, r.Body, r.Link, issueID)
	if err != nil {
		return err
	}
	if muted {
		return nil
	}
	event, err := res.LastInsertId()
	if err != nil {
		return err
	}
	return enqueue(ctx, tx, event, now, r, kind, issue)
}

func isMuted(until sql.NullInt64, now time.Time) bool {
	return until.Valid && until.Int64 > now.UnixMilli()
}

// Record applies a report. Without a key it is one event. With one it moves
// the key's issue along, writing an event only when something changed, so a
// source may repeat itself on every check:
//
//   - A problem new, or back after the flapping window: opened, one event.
//   - A problem going on: counted; one event only if its level rose.
//   - Resolved: recorded now, its event held back until the window passes.
//   - A problem back within that window: open again as if it had not left,
//     with no event at all, so flapping stays quiet.
//   - Resolved with no open issue: nothing, which is the usual case.
func (s *SQLite) Record(ctx context.Context, r Report, now time.Time) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if err := record(ctx, tx, r, now); err != nil {
		return err
	}
	return tx.Commit()
}

func record(ctx context.Context, tx *sql.Tx, r Report, now time.Time) error {
	if r.Key == "" {
		return emit(ctx, tx, now, now, r, KindMessage, 0, false)
	}
	var id, pending int64
	var state, level string
	var mutedUntil sql.NullInt64
	err := tx.QueryRowContext(ctx, `SELECT id, state, level, resolve_pending, muted_until FROM notify_issues WHERE source = ? AND key = ?`, r.Source, r.Key).
		Scan(&id, &state, &level, &pending, &mutedUntil)
	missing := errors.Is(err, sql.ErrNoRows)
	if err != nil && !missing {
		return err
	}
	muted := isMuted(mutedUntil, now)
	if r.Resolved {
		if missing || state != StateOpen {
			return nil
		}
		_, err := tx.ExecContext(ctx, `UPDATE notify_issues SET state = ?, resolved_at = ?, resolve_pending = 1 WHERE id = ?`, StateResolved, now.UnixMilli(), id)
		return err
	}
	switch {
	case missing:
		res, err := tx.ExecContext(ctx, `INSERT INTO notify_issues (source, key, state, level, title, body, link, first_seen, last_seen, occurrences)
			VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, 1)`, r.Source, r.Key, StateOpen, r.Level, r.Title, r.Body, r.Link, now.UnixMilli(), now.UnixMilli())
		if err != nil {
			return err
		}
		id, err = res.LastInsertId()
		if err != nil {
			return err
		}
		return emit(ctx, tx, now, now, r, KindOpened, id, false)
	case state == StateResolved && pending == 0:
		if _, err := tx.ExecContext(ctx, `UPDATE notify_issues SET state = ?, level = ?, title = ?, body = ?, link = ?, first_seen = ?, last_seen = ?,
			resolved_at = NULL, occurrences = 1 WHERE id = ?`, StateOpen, r.Level, r.Title, r.Body, r.Link, now.UnixMilli(), now.UnixMilli(), id); err != nil {
			return err
		}
		return emit(ctx, tx, now, now, r, KindOpened, id, muted)
	}
	// Going on, or back before its resolution was announced.
	if _, err := tx.ExecContext(ctx, `UPDATE notify_issues SET state = ?, level = ?, title = ?, body = ?, link = ?, last_seen = ?,
		resolved_at = NULL, resolve_pending = 0, occurrences = occurrences + 1 WHERE id = ?`,
		StateOpen, r.Level, r.Title, r.Body, r.Link, now.UnixMilli(), id); err != nil {
		return err
	}
	if Rank(r.Level) > Rank(level) {
		return emit(ctx, tx, now, now, r, KindEscalated, id, muted)
	}
	return nil
}

// FlushResolved writes the events of issues resolved longer ago than window,
// dated when they were resolved.
func (s *SQLite) FlushResolved(ctx context.Context, window time.Duration, now time.Time) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	rows, err := tx.QueryContext(ctx, `SELECT id, source, key, title, link, resolved_at, muted_until FROM notify_issues
		WHERE state = ? AND resolve_pending = 1 AND resolved_at <= ?`, StateResolved, now.Add(-window).UnixMilli())
	if err != nil {
		return err
	}
	type due struct {
		id     int64
		report Report
		at     int64
		muted  sql.NullInt64
	}
	var list []due
	for rows.Next() {
		var d due
		if err := rows.Scan(&d.id, &d.report.Source, &d.report.Key, &d.report.Title, &d.report.Link, &d.at, &d.muted); err != nil {
			rows.Close()
			return err
		}
		d.report.Level = LevelInfo
		list = append(list, d)
	}
	if err := rows.Close(); err != nil {
		return err
	}
	for _, d := range list {
		if err := emit(ctx, tx, time.UnixMilli(d.at), now, d.report, KindResolved, d.id, isMuted(d.muted, now)); err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx, `UPDATE notify_issues SET resolve_pending = 0 WHERE id = ?`, d.id); err != nil {
			return err
		}
	}
	return tx.Commit()
}

// CloseIssue resolves an open issue by hand, at once: for one whose source is
// gone and will never say so. If the source reports it again, it reopens.
func (s *SQLite) CloseIssue(ctx context.Context, id int64, now time.Time) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	var r Report
	var state string
	var mutedUntil sql.NullInt64
	err = tx.QueryRowContext(ctx, `SELECT source, key, state, title, link, muted_until FROM notify_issues WHERE id = ?`, id).Scan(&r.Source, &r.Key, &state, &r.Title, &r.Link, &mutedUntil)
	if errors.Is(err, sql.ErrNoRows) {
		return ErrNotFound
	}
	if err != nil {
		return err
	}
	if state != StateOpen {
		return nil
	}
	if _, err := tx.ExecContext(ctx, `UPDATE notify_issues SET state = ?, resolved_at = ?, resolve_pending = 0 WHERE id = ?`, StateResolved, now.UnixMilli(), id); err != nil {
		return err
	}
	r.Level = LevelInfo
	if err := emit(ctx, tx, now, now, r, KindResolved, id, isMuted(mutedUntil, now)); err != nil {
		return err
	}
	return tx.Commit()
}

// EventQuery filters Events; zero values match everything.
type EventQuery struct {
	Level, Source, Search string
	Before                int64 // only events with a lower id
	Limit                 int
}

// Events lists events newest first.
func (s *SQLite) Events(ctx context.Context, q EventQuery) ([]Event, error) {
	where, args := []string{"1 = 1"}, []any{}
	if q.Level != "" {
		where, args = append(where, "level = ?"), append(args, q.Level)
	}
	if q.Source != "" {
		where, args = append(where, "source = ?"), append(args, q.Source)
	}
	if q.Before > 0 {
		where, args = append(where, "id < ?"), append(args, q.Before)
	}
	if q.Search != "" {
		like := "%" + strings.NewReplacer(`\`, `\\`, `%`, `\%`, `_`, `\_`).Replace(q.Search) + "%"
		where, args = append(where, `(title LIKE ? ESCAPE '\' OR body LIKE ? ESCAPE '\')`), append(args, like, like)
	}
	rows, err := s.db.QueryContext(ctx, `SELECT id, created_at, source, level, kind, title, body, link, COALESCE(issue_id, 0)
		FROM notify_events WHERE `+strings.Join(where, " AND ")+` ORDER BY id DESC LIMIT ?`, append(args, q.Limit)...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	list := []Event{}
	for rows.Next() {
		var e Event
		var at int64
		if err := rows.Scan(&e.ID, &at, &e.Source, &e.Level, &e.Kind, &e.Title, &e.Body, &e.Link, &e.IssueID); err != nil {
			return nil, err
		}
		e.Created = time.UnixMilli(at)
		list = append(list, e)
	}
	return list, rows.Err()
}

// Sources lists who has sent events.
func (s *SQLite) Sources(ctx context.Context) ([]string, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT DISTINCT source FROM notify_events ORDER BY source`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	list := []string{}
	for rows.Next() {
		var source string
		if err := rows.Scan(&source); err != nil {
			return nil, err
		}
		list = append(list, source)
	}
	return list, rows.Err()
}

// Issues lists the issues in a state, most severe first, then longest going.
func (s *SQLite) Issues(ctx context.Context, state string) ([]Issue, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT id, source, key, state, level, title, body, link, first_seen, last_seen, resolved_at, occurrences, muted_until
		FROM notify_issues WHERE state = ?
		ORDER BY CASE level WHEN 'critical' THEN 0 WHEN 'warning' THEN 1 WHEN 'info' THEN 2 ELSE 3 END, first_seen`, state)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	list := []Issue{}
	for rows.Next() {
		var i Issue
		var first, last int64
		var resolved, muted sql.NullInt64
		if err := rows.Scan(&i.ID, &i.Source, &i.Key, &i.State, &i.Level, &i.Title, &i.Body, &i.Link, &first, &last, &resolved, &i.Occurrences, &muted); err != nil {
			return nil, err
		}
		i.FirstSeen, i.LastSeen = time.UnixMilli(first), time.UnixMilli(last)
		if resolved.Valid {
			at := time.UnixMilli(resolved.Int64)
			i.ResolvedAt = &at
		}
		if muted.Valid {
			at := time.UnixMilli(muted.Int64)
			i.MutedUntil = &at
		}
		list = append(list, i)
	}
	return list, rows.Err()
}

// Retention says how long events of each level are kept, and how many at most.
type Retention struct {
	Ages map[string]time.Duration
	Max  int
}

// Prune removes old events, and issues resolved longer ago than the longest
// age. The events of open issues always stay.
func (s *SQLite) Prune(ctx context.Context, keep Retention, now time.Time) error {
	const closed = `(issue_id IS NULL OR issue_id NOT IN (SELECT id FROM notify_issues WHERE state = 'open'))`
	var longest time.Duration
	for level, age := range keep.Ages {
		longest = max(longest, age)
		if _, err := s.db.ExecContext(ctx, `DELETE FROM notify_events WHERE level = ? AND created_at < ? AND `+closed, level, now.Add(-age).UnixMilli()); err != nil {
			return err
		}
	}
	if keep.Max > 0 {
		if _, err := s.db.ExecContext(ctx, `DELETE FROM notify_events WHERE `+closed+` AND id <= (
			SELECT id FROM notify_events ORDER BY id DESC LIMIT 1 OFFSET ?)`, keep.Max); err != nil {
			return err
		}
	}
	_, err := s.db.ExecContext(ctx, `DELETE FROM notify_issues WHERE state = 'resolved' AND resolve_pending = 0 AND resolved_at < ?`, now.Add(-longest).UnixMilli())
	return err
}

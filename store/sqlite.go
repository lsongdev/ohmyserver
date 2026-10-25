// Package store keeps jobs and their run history in SQLite.
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

var (
	// ErrNotFound reports a job or run that does not exist.
	ErrNotFound = errors.New("not found")
	// ErrConflict reports a duplicate job name.
	ErrConflict = errors.New("a job with this name exists")
)

// Job is a scheduled command.
type Job struct {
	ID       int64  `json:"id"`
	Name     string `json:"name"`
	Schedule string `json:"schedule"`
	// Timezone is an IANA name the schedule is read in; empty is the
	// server's local time.
	Timezone  string    `json:"timezone"`
	Target    string    `json:"target"` // runner target ID, e.g. "local", "ssh:3"
	Command   string    `json:"command"`
	Dir       string    `json:"dir"`
	Timeout   int       `json:"timeout"` // seconds; 0 is no limit
	Enabled   bool      `json:"enabled"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

// Run statuses.
const (
	StatusRunning     = "running"
	StatusSucceeded   = "succeeded"
	StatusFailed      = "failed"
	StatusTimeout     = "timeout"
	StatusCanceled    = "canceled"
	StatusInterrupted = "interrupted" // the program stopped during the run
)

// Run is one execution of a job.
type Run struct {
	ID         int64     `json:"id"`
	JobID      int64     `json:"job_id"`
	Trigger    string    `json:"trigger"` // schedule | manual
	Status     string    `json:"status"`
	ExitCode   int       `json:"exit_code"`
	Error      string    `json:"error,omitempty"`
	Output     string    `json:"output,omitempty"`
	StartedAt  time.Time `json:"started_at"`
	FinishedAt time.Time `json:"finished_at,omitzero"`
}

// SQLite stores jobs and runs. Its tables are prefixed "cron_", so the
// database can be shared with other modules.
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

// New migrates the cron_ tables in db, which the caller owns and may share
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

// migrations are applied in order; cron_schema records how many have run.
// Append new migrations, never edit old ones.
var migrations = []string{`
CREATE TABLE cron_jobs (
	id         INTEGER PRIMARY KEY,
	name       TEXT NOT NULL UNIQUE,
	schedule   TEXT NOT NULL,
	timezone   TEXT NOT NULL DEFAULT '',
	target     TEXT NOT NULL,
	command    TEXT NOT NULL,
	dir        TEXT NOT NULL DEFAULT '',
	timeout    INTEGER NOT NULL DEFAULT 0,
	enabled    INTEGER NOT NULL,
	created_at INTEGER NOT NULL, -- unix nanoseconds
	updated_at INTEGER NOT NULL
);
CREATE TABLE cron_runs (
	id          INTEGER PRIMARY KEY,
	job_id      INTEGER NOT NULL REFERENCES cron_jobs (id) ON DELETE CASCADE,
	trigger     TEXT NOT NULL,
	status      TEXT NOT NULL,
	exit_code   INTEGER NOT NULL DEFAULT 0,
	error       TEXT NOT NULL DEFAULT '',
	output      TEXT NOT NULL DEFAULT '',
	started_at  INTEGER NOT NULL,
	finished_at INTEGER NOT NULL DEFAULT 0
);
CREATE INDEX cron_runs_by_job ON cron_runs (job_id, id)`}

func (s *SQLite) migrate(ctx context.Context) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err := tx.ExecContext(ctx, `CREATE TABLE IF NOT EXISTS cron_schema (version INTEGER NOT NULL)`); err != nil {
		return err
	}
	var version int
	if err := tx.QueryRowContext(ctx, `SELECT COALESCE(MAX(version), 0) FROM cron_schema`).Scan(&version); err != nil {
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
	if _, err := tx.ExecContext(ctx, `DELETE FROM cron_schema`); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, `INSERT INTO cron_schema (version) VALUES (?)`, len(migrations)); err != nil {
		return err
	}
	return tx.Commit()
}

const jobColumns = `id, name, schedule, timezone, target, command, dir, timeout, enabled, created_at, updated_at`

func scanJob(row interface{ Scan(...any) error }) (Job, error) {
	var j Job
	var created, updated int64
	err := row.Scan(&j.ID, &j.Name, &j.Schedule, &j.Timezone, &j.Target, &j.Command, &j.Dir, &j.Timeout, &j.Enabled, &created, &updated)
	if errors.Is(err, sql.ErrNoRows) {
		return j, ErrNotFound
	}
	j.CreatedAt, j.UpdatedAt = time.Unix(0, created), time.Unix(0, updated)
	return j, err
}

// Jobs returns every job by name.
func (s *SQLite) Jobs(ctx context.Context) ([]Job, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT `+jobColumns+` FROM cron_jobs ORDER BY name`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	jobs := []Job{}
	for rows.Next() {
		j, err := scanJob(rows)
		if err != nil {
			return nil, err
		}
		jobs = append(jobs, j)
	}
	return jobs, rows.Err()
}

// Job returns one job.
func (s *SQLite) Job(ctx context.Context, id int64) (Job, error) {
	return scanJob(s.db.QueryRowContext(ctx, `SELECT `+jobColumns+` FROM cron_jobs WHERE id = ?`, id))
}

// CreateJob inserts j and sets its ID and timestamps.
func (s *SQLite) CreateJob(ctx context.Context, j *Job) error {
	now := time.Now()
	res, err := s.db.ExecContext(ctx, `INSERT INTO cron_jobs (name, schedule, timezone, target, command, dir, timeout, enabled, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`, j.Name, j.Schedule, j.Timezone, j.Target, j.Command, j.Dir, j.Timeout, j.Enabled, now.UnixNano(), now.UnixNano())
	if err != nil {
		return conflict(err)
	}
	j.ID, err = res.LastInsertId()
	j.CreatedAt, j.UpdatedAt = now, now
	return err
}

// UpdateJob replaces j's fields.
func (s *SQLite) UpdateJob(ctx context.Context, j *Job) error {
	now := time.Now()
	res, err := s.db.ExecContext(ctx, `UPDATE cron_jobs SET name = ?, schedule = ?, timezone = ?, target = ?, command = ?, dir = ?,
		timeout = ?, enabled = ?, updated_at = ? WHERE id = ?`,
		j.Name, j.Schedule, j.Timezone, j.Target, j.Command, j.Dir, j.Timeout, j.Enabled, now.UnixNano(), j.ID)
	if err != nil {
		return conflict(err)
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrNotFound
	}
	j.UpdatedAt = now
	return nil
}

// DeleteJob removes a job and its runs.
func (s *SQLite) DeleteJob(ctx context.Context, id int64) error {
	_, err := s.db.ExecContext(ctx, `DELETE FROM cron_jobs WHERE id = ?`, id)
	return err
}

const runColumns = `id, job_id, trigger, status, exit_code, error, started_at, finished_at`

func scanRun(row interface{ Scan(...any) error }, withOutput bool) (Run, error) {
	var r Run
	var started, finished int64
	dst := []any{&r.ID, &r.JobID, &r.Trigger, &r.Status, &r.ExitCode, &r.Error, &started, &finished}
	if withOutput {
		dst = append(dst, &r.Output)
	}
	err := row.Scan(dst...)
	if errors.Is(err, sql.ErrNoRows) {
		return r, ErrNotFound
	}
	r.StartedAt = time.Unix(0, started)
	if finished > 0 {
		r.FinishedAt = time.Unix(0, finished)
	}
	return r, err
}

// StartRun records a run that has started.
func (s *SQLite) StartRun(ctx context.Context, r *Run) error {
	res, err := s.db.ExecContext(ctx, `INSERT INTO cron_runs (job_id, trigger, status, started_at) VALUES (?, ?, ?, ?)`,
		r.JobID, r.Trigger, r.Status, r.StartedAt.UnixNano())
	if err != nil {
		return err
	}
	r.ID, err = res.LastInsertId()
	return err
}

// FinishRun records a run's outcome and keeps the job's latest keep runs.
func (s *SQLite) FinishRun(ctx context.Context, r Run, keep int) error {
	if _, err := s.db.ExecContext(ctx, `UPDATE cron_runs SET status = ?, exit_code = ?, error = ?, output = ?, finished_at = ? WHERE id = ?`,
		r.Status, r.ExitCode, r.Error, r.Output, r.FinishedAt.UnixNano(), r.ID); err != nil {
		return err
	}
	_, err := s.db.ExecContext(ctx, `DELETE FROM cron_runs WHERE job_id = ? AND id NOT IN
		(SELECT id FROM cron_runs WHERE job_id = ? ORDER BY id DESC LIMIT ?)`, r.JobID, r.JobID, keep)
	return err
}

// Runs returns a job's latest runs, newest first, without their output.
func (s *SQLite) Runs(ctx context.Context, jobID int64, limit int) ([]Run, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT `+runColumns+` FROM cron_runs WHERE job_id = ? ORDER BY id DESC LIMIT ?`, jobID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	runs := []Run{}
	for rows.Next() {
		r, err := scanRun(rows, false)
		if err != nil {
			return nil, err
		}
		runs = append(runs, r)
	}
	return runs, rows.Err()
}

// LastRuns returns each job's latest run, by job ID.
func (s *SQLite) LastRuns(ctx context.Context) (map[int64]Run, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT `+runColumns+` FROM cron_runs WHERE id IN (SELECT MAX(id) FROM cron_runs GROUP BY job_id)`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[int64]Run{}
	for rows.Next() {
		r, err := scanRun(rows, false)
		if err != nil {
			return nil, err
		}
		out[r.JobID] = r
	}
	return out, rows.Err()
}

// Run returns one run with its output.
func (s *SQLite) Run(ctx context.Context, id int64) (Run, error) {
	return scanRun(s.db.QueryRowContext(ctx, `SELECT `+runColumns+`, output FROM cron_runs WHERE id = ?`, id), true)
}

// InterruptRuns marks runs still recorded as running as interrupted: the
// program stopped while they ran.
func (s *SQLite) InterruptRuns(ctx context.Context) error {
	_, err := s.db.ExecContext(ctx, `UPDATE cron_runs SET status = ?, finished_at = ? WHERE status = ?`,
		StatusInterrupted, time.Now().UnixNano(), StatusRunning)
	return err
}

func conflict(err error) error {
	if strings.Contains(err.Error(), "UNIQUE constraint failed") {
		return ErrConflict
	}
	return err
}

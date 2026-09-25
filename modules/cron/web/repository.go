package web

import (
	"context"
	"database/sql"
	cronmodule "github.com/lsongdev/ohmyserver/modules/cron"
	"time"
)

// Repository owns only cron_jobs and cron_job_executions.
type Repository struct{ db *sql.DB }

func NewRepository(db *sql.DB) *Repository { return &Repository{db: db} }

func (r *Repository) Initialize(ctx context.Context) error {
	_, err := r.db.ExecContext(ctx, `
CREATE TABLE IF NOT EXISTS cron_jobs (
 id INTEGER PRIMARY KEY AUTOINCREMENT, name TEXT NOT NULL,
 type TEXT NOT NULL DEFAULT 'cron', schedule TEXT NOT NULL, command TEXT NOT NULL,
 working_dir TEXT DEFAULT '', timeout INTEGER DEFAULT 0, enabled INTEGER DEFAULT 1,
 last_run DATETIME, next_run DATETIME, created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
 updated_at DATETIME DEFAULT CURRENT_TIMESTAMP
);
CREATE INDEX IF NOT EXISTS idx_cron_jobs_name ON cron_jobs(name);
CREATE INDEX IF NOT EXISTS idx_cron_jobs_enabled ON cron_jobs(enabled);
CREATE TABLE IF NOT EXISTS cron_job_executions (
 id INTEGER PRIMARY KEY AUTOINCREMENT, job_id INTEGER NOT NULL,
 status TEXT NOT NULL DEFAULT 'running', output TEXT, error TEXT,
 exit_code INTEGER DEFAULT 0, started_at DATETIME DEFAULT CURRENT_TIMESTAMP,
 completed_at DATETIME, created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
 FOREIGN KEY (job_id) REFERENCES cron_jobs(id) ON DELETE CASCADE
);
CREATE INDEX IF NOT EXISTS idx_cron_executions_job_id ON cron_job_executions(job_id);
CREATE INDEX IF NOT EXISTS idx_cron_executions_status ON cron_job_executions(status);`)
	return err
}

func (r *Repository) RecoverInterruptedExecutions(ctx context.Context) error {
	_, err := r.db.ExecContext(ctx, `UPDATE cron_job_executions SET status='interrupted', error='Cron process stopped before this execution completed', completed_at=COALESCE(completed_at,CURRENT_TIMESTAMP) WHERE status='running'`)
	return err
}

const jobColumns = `id, name, type, schedule, command, working_dir, timeout, enabled, last_run, next_run, created_at, updated_at`
const executionColumns = `id, job_id, status, output, error, exit_code, started_at, completed_at, created_at`

func (r *Repository) Create(ctx context.Context, in Input) (Job, error) {
	now := time.Now()
	var next *time.Time
	var err error
	if in.Type == "cron" {
		next, err = cronmodule.ParseCronExpression(in.Schedule)
	} else if in.Type == "once" {
		var t time.Time
		t, err = time.Parse(time.RFC3339, in.Schedule)
		if err == nil && t.After(now) {
			next = &t
		}
	}
	if err != nil {
		return Job{}, err
	}
	result, err := r.db.ExecContext(ctx, `INSERT INTO cron_jobs (name,type,schedule,command,working_dir,timeout,enabled,next_run,created_at,updated_at) VALUES (?,?,?,?,?,?,1,?,?,?)`, in.Name, in.Type, in.Schedule, in.Command, in.WorkingDir, in.Timeout, next, now, now)
	if err != nil {
		return Job{}, err
	}
	id, err := result.LastInsertId()
	if err != nil {
		return Job{}, err
	}
	return Job{ID: id, Name: in.Name, Type: in.Type, Schedule: in.Schedule, Command: in.Command, WorkingDir: in.WorkingDir, Timeout: in.Timeout, Enabled: true, NextRun: next, CreatedAt: now, UpdatedAt: now}, nil
}

func (r *Repository) Get(ctx context.Context, id int64) (Job, error) {
	return scanJob(r.db.QueryRowContext(ctx, `SELECT `+jobColumns+` FROM cron_jobs WHERE id=?`, id))
}
func (r *Repository) List(ctx context.Context) ([]Job, error)    { return r.list(ctx, false) }
func (r *Repository) Enabled(ctx context.Context) ([]Job, error) { return r.list(ctx, true) }
func (r *Repository) list(ctx context.Context, enabled bool) ([]Job, error) {
	query := `SELECT ` + jobColumns + ` FROM cron_jobs`
	if enabled {
		query += ` WHERE enabled=1`
	}
	query += ` ORDER BY created_at DESC`
	rows, err := r.db.QueryContext(ctx, query)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := make([]Job, 0)
	for rows.Next() {
		item, err := scanJob(rows)
		if err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	return items, rows.Err()
}
func (r *Repository) Update(ctx context.Context, id int64, in Input) error {
	now := time.Now()
	var next any
	if in.Type == "cron" {
		t, err := cronmodule.ParseCronExpression(in.Schedule)
		if err != nil {
			return err
		}
		next = t
	} else if in.Type == "once" {
		t, err := time.Parse(time.RFC3339, in.Schedule)
		if err != nil {
			return err
		}
		if t.After(now) {
			next = t
		}
	}
	_, err := r.db.ExecContext(ctx, `UPDATE cron_jobs SET name=?,type=?,schedule=?,command=?,working_dir=?,timeout=?,next_run=?,updated_at=? WHERE id=?`, in.Name, in.Type, in.Schedule, in.Command, in.WorkingDir, in.Timeout, next, now, id)
	return err
}
func (r *Repository) Delete(ctx context.Context, id int64) error {
	_, err := r.db.ExecContext(ctx, `DELETE FROM cron_jobs WHERE id=?`, id)
	return err
}
func (r *Repository) SetEnabled(ctx context.Context, id int64, enabled bool) error {
	_, err := r.db.ExecContext(ctx, `UPDATE cron_jobs SET enabled=?,updated_at=? WHERE id=?`, enabled, time.Now(), id)
	return err
}
func (r *Repository) UpdateRunTimes(ctx context.Context, id int64, last time.Time, next *time.Time) error {
	_, err := r.db.ExecContext(ctx, `UPDATE cron_jobs SET last_run=?,next_run=?,updated_at=? WHERE id=?`, last, next, time.Now(), id)
	return err
}
func (r *Repository) UpdateLastRun(ctx context.Context, id int64, last time.Time) error {
	_, err := r.db.ExecContext(ctx, `UPDATE cron_jobs SET last_run=?,updated_at=? WHERE id=?`, last, time.Now(), id)
	return err
}
func (r *Repository) StartExecution(ctx context.Context, jobID int64) (Execution, error) {
	now := time.Now()
	result, err := r.db.ExecContext(ctx, `INSERT INTO cron_job_executions (job_id,status,started_at,created_at) VALUES (?,'running',?,?)`, jobID, now, now)
	if err != nil {
		return Execution{}, err
	}
	id, err := result.LastInsertId()
	if err != nil {
		return Execution{}, err
	}
	return Execution{ID: id, JobID: jobID, Status: "running", StartedAt: now, CreatedAt: now}, nil
}
func (r *Repository) CompleteExecution(ctx context.Context, id int64, status, output, errorMsg string, exitCode int) error {
	_, err := r.db.ExecContext(ctx, `UPDATE cron_job_executions SET status=?,output=?,error=?,exit_code=?,completed_at=? WHERE id=?`, status, output, errorMsg, exitCode, time.Now(), id)
	return err
}
func (r *Repository) Executions(ctx context.Context, jobID int64, limit int) ([]Execution, error) {
	return r.executionList(ctx, `SELECT `+executionColumns+` FROM cron_job_executions WHERE job_id=? ORDER BY created_at DESC LIMIT ?`, jobID, limit)
}
func (r *Repository) RecentExecutions(ctx context.Context, limit int) ([]Execution, error) {
	return r.executionList(ctx, `SELECT `+executionColumns+` FROM cron_job_executions ORDER BY created_at DESC LIMIT ?`, limit)
}
func (r *Repository) executionList(ctx context.Context, query string, args ...any) ([]Execution, error) {
	rows, err := r.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := make([]Execution, 0)
	for rows.Next() {
		item, err := scanExecution(rows)
		if err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	return items, rows.Err()
}

type scanner interface{ Scan(...any) error }

func scanJob(row scanner) (Job, error) {
	var x Job
	var last, next, created, updated sql.NullString
	err := row.Scan(&x.ID, &x.Name, &x.Type, &x.Schedule, &x.Command, &x.WorkingDir, &x.Timeout, &x.Enabled, &last, &next, &created, &updated)
	if err != nil {
		return Job{}, err
	}
	x.LastRun = parseDBTime(last)
	x.NextRun = parseDBTime(next)
	x.CreatedAt = parseTimeString(created.String)
	x.UpdatedAt = parseTimeString(updated.String)
	return x, nil
}
func scanExecution(row scanner) (Execution, error) {
	var x Execution
	var start, complete, created sql.NullString
	err := row.Scan(&x.ID, &x.JobID, &x.Status, &x.Output, &x.Error, &x.ExitCode, &start, &complete, &created)
	if err != nil {
		return Execution{}, err
	}
	x.StartedAt = parseTimeString(start.String)
	x.CompletedAt = parseDBTime(complete)
	x.CreatedAt = parseTimeString(created.String)
	return x, nil
}
func parseDBTime(value sql.NullString) *time.Time {
	if !value.Valid {
		return nil
	}
	t := parseTimeString(value.String)
	if t.IsZero() {
		return nil
	}
	return &t
}
func parseTimeString(value string) time.Time {
	if value == "" {
		return time.Time{}
	}
	for _, layout := range []string{time.RFC3339Nano, "2006-01-02 15:04:05.999999999-07:00", "2006-01-02 15:04:05.999999-07:00", "2006-01-02 15:04:05-07:00", "2006-01-02 15:04:05.999999999", "2006-01-02 15:04:05.999999", "2006-01-02 15:04:05"} {
		if t, err := time.Parse(layout, value); err == nil {
			return t
		}
	}
	return time.Time{}
}

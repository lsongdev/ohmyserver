// Package store keeps repositories, builds and their step logs in SQLite.
package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	_ "modernc.org/sqlite" // registers the "sqlite" driver for Open
)

var (
	// ErrNotFound reports a repository, build or step that does not exist.
	ErrNotFound = errors.New("not found")
	// ErrConflict reports a duplicate repository name.
	ErrConflict = errors.New("a repository with this name exists")
)

// Repo is a repository ci-go builds.
type Repo struct {
	ID            int64             `json:"id"`
	Name          string            `json:"name"`
	URL           string            `json:"url"`    // git clone URL
	Branch        string            `json:"branch"` // built by manual runs by default
	ConfigPath    string            `json:"config_path"`
	Token         string            `json:"-"` // for HTTPS clones of private repositories
	WebhookSecret string            `json:"-"`
	Secrets       map[string]string `json:"-"`       // passed to steps that ask for them
	Timeout       int               `json:"timeout"` // minutes per build
	CreatedAt     time.Time         `json:"created_at"`
	UpdatedAt     time.Time         `json:"updated_at"`
}

// Build is one run of a repository's pipeline.
type Build struct {
	ID         int64     `json:"id"`
	RepoID     int64     `json:"repo_id"`
	Number     int       `json:"number"` // per repository
	Event      string    `json:"event"`  // push | manual
	Branch     string    `json:"branch"`
	Commit     string    `json:"commit"` // SHA, once known
	Message    string    `json:"message"`
	Author     string    `json:"author"`
	Status     string    `json:"status"`
	Error      string    `json:"error,omitempty"`
	CreatedAt  time.Time `json:"created_at"`
	StartedAt  time.Time `json:"started_at,omitzero"`
	FinishedAt time.Time `json:"finished_at,omitzero"`
	Steps      []Step    `json:"steps,omitempty"`
}

// Step is one step of a build.
type Step struct {
	ID         int64     `json:"id"`
	BuildID    int64     `json:"build_id"`
	Position   int       `json:"position"`
	Name       string    `json:"name"`
	Status     string    `json:"status"`
	ExitCode   int       `json:"exit_code"`
	StartedAt  time.Time `json:"started_at,omitzero"`
	FinishedAt time.Time `json:"finished_at,omitzero"`
}

// SQLite stores repositories and builds. Its tables are prefixed "ci_",
// so the database can be shared with other modules.
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

// New migrates the ci_ tables in db, which the caller owns and may share
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

// migrations are applied in order; ci_schema records how many have run.
// Append new migrations, never edit old ones.
var migrations = []string{`
CREATE TABLE ci_repos (
	id             INTEGER PRIMARY KEY,
	name           TEXT NOT NULL UNIQUE,
	url            TEXT NOT NULL,
	branch         TEXT NOT NULL,
	config_path    TEXT NOT NULL,
	token          TEXT NOT NULL DEFAULT '',
	webhook_secret TEXT NOT NULL,
	secrets        TEXT NOT NULL DEFAULT '{}', -- JSON object
	timeout        INTEGER NOT NULL,
	created_at     INTEGER NOT NULL,          -- unix nanoseconds
	updated_at     INTEGER NOT NULL
);
CREATE TABLE ci_builds (
	id          INTEGER PRIMARY KEY,
	repo_id     INTEGER NOT NULL REFERENCES ci_repos (id) ON DELETE CASCADE,
	number      INTEGER NOT NULL,
	event       TEXT NOT NULL,
	branch      TEXT NOT NULL,
	commit_sha  TEXT NOT NULL DEFAULT '',
	message     TEXT NOT NULL DEFAULT '',
	author      TEXT NOT NULL DEFAULT '',
	status      TEXT NOT NULL,
	error       TEXT NOT NULL DEFAULT '',
	created_at  INTEGER NOT NULL,
	started_at  INTEGER NOT NULL DEFAULT 0,
	finished_at INTEGER NOT NULL DEFAULT 0,
	UNIQUE (repo_id, number)
);
CREATE TABLE ci_steps (
	id          INTEGER PRIMARY KEY,
	build_id    INTEGER NOT NULL REFERENCES ci_builds (id) ON DELETE CASCADE,
	position    INTEGER NOT NULL,
	name        TEXT NOT NULL,
	status      TEXT NOT NULL,
	exit_code   INTEGER NOT NULL DEFAULT 0,
	log         TEXT NOT NULL DEFAULT '',
	started_at  INTEGER NOT NULL DEFAULT 0,
	finished_at INTEGER NOT NULL DEFAULT 0,
	UNIQUE (build_id, position)
)`}

func (s *SQLite) migrate(ctx context.Context) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err := tx.ExecContext(ctx, `CREATE TABLE IF NOT EXISTS ci_schema (version INTEGER NOT NULL)`); err != nil {
		return err
	}
	var version int
	if err := tx.QueryRowContext(ctx, `SELECT COALESCE(MAX(version), 0) FROM ci_schema`).Scan(&version); err != nil {
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
	if _, err := tx.ExecContext(ctx, `DELETE FROM ci_schema`); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, `INSERT INTO ci_schema (version) VALUES (?)`, len(migrations)); err != nil {
		return err
	}
	return tx.Commit()
}

func unix(t time.Time) int64 {
	if t.IsZero() {
		return 0
	}
	return t.UnixNano()
}

func fromUnix(n int64) time.Time {
	if n == 0 {
		return time.Time{}
	}
	return time.Unix(0, n)
}

// ---- repositories ----

const repoColumns = `id, name, url, branch, config_path, token, webhook_secret, secrets, timeout, created_at, updated_at`

func scanRepo(row interface{ Scan(...any) error }) (Repo, error) {
	var r Repo
	var secrets string
	var created, updated int64
	err := row.Scan(&r.ID, &r.Name, &r.URL, &r.Branch, &r.ConfigPath, &r.Token, &r.WebhookSecret, &secrets, &r.Timeout, &created, &updated)
	if errors.Is(err, sql.ErrNoRows) {
		return r, ErrNotFound
	}
	json.Unmarshal([]byte(secrets), &r.Secrets)
	if r.Secrets == nil {
		r.Secrets = map[string]string{}
	}
	r.CreatedAt, r.UpdatedAt = fromUnix(created), fromUnix(updated)
	return r, err
}

// Repos returns every repository by name.
func (s *SQLite) Repos(ctx context.Context) ([]Repo, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT `+repoColumns+` FROM ci_repos ORDER BY name`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	repos := []Repo{}
	for rows.Next() {
		r, err := scanRepo(rows)
		if err != nil {
			return nil, err
		}
		repos = append(repos, r)
	}
	return repos, rows.Err()
}

// Repo returns one repository.
func (s *SQLite) Repo(ctx context.Context, id int64) (Repo, error) {
	return scanRepo(s.db.QueryRowContext(ctx, `SELECT `+repoColumns+` FROM ci_repos WHERE id = ?`, id))
}

// CreateRepo inserts r and sets its ID and timestamps.
func (s *SQLite) CreateRepo(ctx context.Context, r *Repo) error {
	secrets, _ := json.Marshal(r.Secrets)
	now := time.Now()
	res, err := s.db.ExecContext(ctx, `INSERT INTO ci_repos (name, url, branch, config_path, token, webhook_secret, secrets, timeout, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`, r.Name, r.URL, r.Branch, r.ConfigPath, r.Token, r.WebhookSecret, string(secrets), r.Timeout, now.UnixNano(), now.UnixNano())
	if err != nil {
		return conflict(err)
	}
	r.ID, err = res.LastInsertId()
	r.CreatedAt, r.UpdatedAt = now, now
	return err
}

// UpdateRepo replaces r's fields.
func (s *SQLite) UpdateRepo(ctx context.Context, r *Repo) error {
	secrets, _ := json.Marshal(r.Secrets)
	now := time.Now()
	res, err := s.db.ExecContext(ctx, `UPDATE ci_repos SET name = ?, url = ?, branch = ?, config_path = ?, token = ?, webhook_secret = ?,
		secrets = ?, timeout = ?, updated_at = ? WHERE id = ?`,
		r.Name, r.URL, r.Branch, r.ConfigPath, r.Token, r.WebhookSecret, string(secrets), r.Timeout, now.UnixNano(), r.ID)
	if err != nil {
		return conflict(err)
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrNotFound
	}
	r.UpdatedAt = now
	return nil
}

// DeleteRepo removes a repository and its builds.
func (s *SQLite) DeleteRepo(ctx context.Context, id int64) error {
	_, err := s.db.ExecContext(ctx, `DELETE FROM ci_repos WHERE id = ?`, id)
	return err
}

// ---- builds ----

const buildColumns = `id, repo_id, number, event, branch, commit_sha, message, author, status, error, created_at, started_at, finished_at`

func scanBuild(row interface{ Scan(...any) error }) (Build, error) {
	var b Build
	var created, started, finished int64
	err := row.Scan(&b.ID, &b.RepoID, &b.Number, &b.Event, &b.Branch, &b.Commit, &b.Message, &b.Author, &b.Status, &b.Error, &created, &started, &finished)
	if errors.Is(err, sql.ErrNoRows) {
		return b, ErrNotFound
	}
	b.CreatedAt, b.StartedAt, b.FinishedAt = fromUnix(created), fromUnix(started), fromUnix(finished)
	return b, err
}

// CreateBuild queues a build, numbering it after the repository's last.
func (s *SQLite) CreateBuild(ctx context.Context, b *Build) error {
	b.CreatedAt = time.Now()
	return s.db.QueryRowContext(ctx, `INSERT INTO ci_builds (repo_id, number, event, branch, commit_sha, message, author, status, created_at)
		VALUES (?, (SELECT COALESCE(MAX(number), 0) + 1 FROM ci_builds WHERE repo_id = ?), ?, ?, ?, ?, ?, ?, ?)
		RETURNING id, number`, b.RepoID, b.RepoID, b.Event, b.Branch, b.Commit, b.Message, b.Author, b.Status, b.CreatedAt.UnixNano()).Scan(&b.ID, &b.Number)
}

// UpdateBuild saves a build's state and commit.
func (s *SQLite) UpdateBuild(ctx context.Context, b Build) error {
	_, err := s.db.ExecContext(ctx, `UPDATE ci_builds SET commit_sha = ?, message = ?, author = ?, status = ?, error = ?, started_at = ?, finished_at = ?
		WHERE id = ?`, b.Commit, b.Message, b.Author, b.Status, b.Error, unix(b.StartedAt), unix(b.FinishedAt), b.ID)
	return err
}

// Build returns a build with its steps.
func (s *SQLite) Build(ctx context.Context, id int64) (Build, error) {
	b, err := scanBuild(s.db.QueryRowContext(ctx, `SELECT `+buildColumns+` FROM ci_builds WHERE id = ?`, id))
	if err != nil {
		return b, err
	}
	b.Steps, err = s.Steps(ctx, id)
	return b, err
}

// Builds returns a repository's latest builds, newest first; repoID 0
// lists every repository's.
func (s *SQLite) Builds(ctx context.Context, repoID int64, limit int) ([]Build, error) {
	query, args := `SELECT `+buildColumns+` FROM ci_builds`, []any{}
	if repoID > 0 {
		query += ` WHERE repo_id = ?`
		args = append(args, repoID)
	}
	rows, err := s.db.QueryContext(ctx, query+` ORDER BY id DESC LIMIT ?`, append(args, limit)...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	builds := []Build{}
	for rows.Next() {
		b, err := scanBuild(rows)
		if err != nil {
			return nil, err
		}
		builds = append(builds, b)
	}
	return builds, rows.Err()
}

// LastBuilds returns each repository's latest build, by repository ID.
func (s *SQLite) LastBuilds(ctx context.Context) (map[int64]Build, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT `+buildColumns+` FROM ci_builds WHERE id IN (SELECT MAX(id) FROM ci_builds GROUP BY repo_id)`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[int64]Build{}
	for rows.Next() {
		b, err := scanBuild(rows)
		if err != nil {
			return nil, err
		}
		out[b.RepoID] = b
	}
	return out, rows.Err()
}

// Pending returns queued builds, oldest first.
func (s *SQLite) Pending(ctx context.Context) ([]Build, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT `+buildColumns+` FROM ci_builds WHERE status = 'pending' ORDER BY id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var builds []Build
	for rows.Next() {
		b, err := scanBuild(rows)
		if err != nil {
			return nil, err
		}
		builds = append(builds, b)
	}
	return builds, rows.Err()
}

// Interrupt marks builds and steps left running by a previous process as
// errors.
func (s *SQLite) Interrupt(ctx context.Context) error {
	now := time.Now().UnixNano()
	if _, err := s.db.ExecContext(ctx, `UPDATE ci_steps SET status = 'error', finished_at = ? WHERE status IN ('running', 'pending')
		AND build_id IN (SELECT id FROM ci_builds WHERE status = 'running')`, now); err != nil {
		return err
	}
	_, err := s.db.ExecContext(ctx, `UPDATE ci_builds SET status = 'error', error = 'interrupted: the server stopped during the build', finished_at = ?
		WHERE status = 'running'`, now)
	return err
}

// Prune keeps the latest keep builds of each repository.
func (s *SQLite) Prune(ctx context.Context, repoID int64, keep int) error {
	_, err := s.db.ExecContext(ctx, `DELETE FROM ci_builds WHERE repo_id = ? AND status NOT IN ('pending', 'running') AND id NOT IN
		(SELECT id FROM ci_builds WHERE repo_id = ? ORDER BY id DESC LIMIT ?)`, repoID, repoID, keep)
	return err
}

// ---- steps ----

const stepColumns = `id, build_id, position, name, status, exit_code, started_at, finished_at`

// SetSteps records a build's step names; existing positions keep their
// state.
func (s *SQLite) SetSteps(ctx context.Context, buildID int64, names []string) error {
	for i, name := range names {
		if _, err := s.db.ExecContext(ctx, `INSERT INTO ci_steps (build_id, position, name, status) VALUES (?, ?, ?, 'pending')
			ON CONFLICT (build_id, position) DO NOTHING`, buildID, i, name); err != nil {
			return err
		}
	}
	return nil
}

// UpdateStep saves a step's state; a non-nil log replaces the stored one.
func (s *SQLite) UpdateStep(ctx context.Context, st Step, log *string) error {
	query := `UPDATE ci_steps SET status = ?, exit_code = ?, started_at = ?, finished_at = ?`
	args := []any{st.Status, st.ExitCode, unix(st.StartedAt), unix(st.FinishedAt)}
	if log != nil {
		query += `, log = ?`
		args = append(args, *log)
	}
	_, err := s.db.ExecContext(ctx, query+` WHERE build_id = ? AND position = ?`, append(args, st.BuildID, st.Position)...)
	return err
}

// Steps returns a build's steps in order.
func (s *SQLite) Steps(ctx context.Context, buildID int64) ([]Step, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT `+stepColumns+` FROM ci_steps WHERE build_id = ? ORDER BY position`, buildID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	steps := []Step{}
	for rows.Next() {
		var st Step
		var started, finished int64
		if err := rows.Scan(&st.ID, &st.BuildID, &st.Position, &st.Name, &st.Status, &st.ExitCode, &started, &finished); err != nil {
			return nil, err
		}
		st.StartedAt, st.FinishedAt = fromUnix(started), fromUnix(finished)
		steps = append(steps, st)
	}
	return steps, rows.Err()
}

// StepLog returns a finished step's stored log.
func (s *SQLite) StepLog(ctx context.Context, buildID int64, position int) (string, error) {
	var log string
	err := s.db.QueryRowContext(ctx, `SELECT log FROM ci_steps WHERE build_id = ? AND position = ?`, buildID, position).Scan(&log)
	if errors.Is(err, sql.ErrNoRows) {
		return "", ErrNotFound
	}
	return log, err
}

func conflict(err error) error {
	if strings.Contains(err.Error(), "UNIQUE constraint failed: ci_repos.name") {
		return ErrConflict
	}
	return err
}

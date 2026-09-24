package web

import (
	"context"
	"database/sql"
	"fmt"
	"time"
)

type Repository struct{ db *sql.DB }

func NewRepository(db *sql.DB) *Repository { return &Repository{db: db} }
func (r *Repository) Initialize(ctx context.Context) error {
	_, err := r.db.ExecContext(ctx, `CREATE TABLE IF NOT EXISTS projects(id INTEGER PRIMARY KEY AUTOINCREMENT,name TEXT NOT NULL UNIQUE,description TEXT,repo TEXT DEFAULT '',webhook_secret TEXT NOT NULL DEFAULT '',webhook_branch TEXT NOT NULL DEFAULT 'main',created_at DATETIME DEFAULT CURRENT_TIMESTAMP,updated_at DATETIME DEFAULT CURRENT_TIMESTAMP);CREATE INDEX IF NOT EXISTS idx_projects_name ON projects(name);`)
	if err != nil {
		return err
	}
	for _, column := range []struct{ name, definition string }{{"repo", "TEXT DEFAULT ''"}, {"webhook_secret", "TEXT NOT NULL DEFAULT ''"}, {"webhook_branch", "TEXT NOT NULL DEFAULT 'main'"}} {
		if err := r.ensureColumn(ctx, column.name, column.definition); err != nil {
			return err
		}
	}
	_, err = r.db.ExecContext(ctx, `CREATE TABLE IF NOT EXISTS deployments(id INTEGER PRIMARY KEY AUTOINCREMENT,project_id INTEGER NOT NULL,status TEXT NOT NULL DEFAULT 'pending',docker_image TEXT DEFAULT '',dockerfile TEXT,container_name TEXT DEFAULT '',container_id TEXT DEFAULT '',command TEXT DEFAULT '',"commit" TEXT DEFAULT '',commit_msg TEXT DEFAULT '',author TEXT DEFAULT '',triggered_by TEXT DEFAULT 'Manual',github_delivery_id TEXT,started_at DATETIME,completed_at DATETIME,created_at DATETIME DEFAULT CURRENT_TIMESTAMP,updated_at DATETIME DEFAULT CURRENT_TIMESTAMP,FOREIGN KEY(project_id) REFERENCES projects(id) ON DELETE CASCADE);CREATE INDEX IF NOT EXISTS idx_deployments_project_id ON deployments(project_id);CREATE INDEX IF NOT EXISTS idx_deployments_status ON deployments(status);CREATE TABLE IF NOT EXISTS deployment_logs(id INTEGER PRIMARY KEY AUTOINCREMENT,deployment_id INTEGER NOT NULL,message TEXT NOT NULL,created_at DATETIME DEFAULT CURRENT_TIMESTAMP,FOREIGN KEY(deployment_id) REFERENCES deployments(id) ON DELETE CASCADE);CREATE INDEX IF NOT EXISTS idx_logs_deployment_id ON deployment_logs(deployment_id);`)
	if err != nil {
		return err
	}
	for _, column := range []struct{ name, definition string }{{"container_id", "TEXT DEFAULT ''"}, {"github_delivery_id", "TEXT"}} {
		if err := r.ensureDeploymentColumn(ctx, column.name, column.definition); err != nil {
			return err
		}
	}
	_, err = r.db.ExecContext(ctx, `CREATE UNIQUE INDEX IF NOT EXISTS idx_deployments_github_delivery ON deployments(github_delivery_id) WHERE github_delivery_id IS NOT NULL`)
	if err != nil {
		return err
	}
	return nil
}
func (r *Repository) ensureDeploymentColumn(ctx context.Context, name, definition string) error {
	rows, err := r.db.QueryContext(ctx, `PRAGMA table_info(deployments)`)
	if err != nil {
		return err
	}
	found := false
	for rows.Next() {
		var cid, notnull, pk int
		var column, kind string
		var defaultValue any
		if err := rows.Scan(&cid, &column, &kind, &notnull, &defaultValue, &pk); err != nil {
			rows.Close()
			return err
		}
		if column == name {
			found = true
		}
	}
	if err := rows.Close(); err != nil {
		return err
	}
	if found {
		return nil
	}
	_, err = r.db.ExecContext(ctx, `ALTER TABLE deployments ADD COLUMN `+name+` `+definition)
	return err
}
func (r *Repository) ensureColumn(ctx context.Context, name, definition string) error {
	rows, err := r.db.QueryContext(ctx, `PRAGMA table_info(projects)`)
	if err != nil {
		return err
	}
	found := false
	for rows.Next() {
		var cid, notnull, pk int
		var column, kind string
		var defaultValue any
		if err := rows.Scan(&cid, &column, &kind, &notnull, &defaultValue, &pk); err != nil {
			rows.Close()
			return err
		}
		if column == name {
			found = true
		}
	}
	if err := rows.Close(); err != nil {
		return err
	}
	if found {
		return nil
	}
	_, err = r.db.ExecContext(ctx, `ALTER TABLE projects ADD COLUMN `+name+` `+definition)
	return err
}

const projectColumns = `id,name,description,repo,webhook_secret,webhook_branch,created_at,updated_at`

func scanProject(row interface{ Scan(...any) error }) (*Project, error) {
	var x Project
	var created, updated string
	if err := row.Scan(&x.ID, &x.Name, &x.Description, &x.Repo, &x.WebhookSecret, &x.WebhookBranch, &created, &updated); err != nil {
		return nil, err
	}
	x.CreatedAt = parseTime(created)
	x.UpdatedAt = parseTime(updated)
	return &x, nil
}
func parseTime(value string) time.Time {
	for _, layout := range []string{time.RFC3339Nano, "2006-01-02 15:04:05.999999999-07:00", "2006-01-02 15:04:05.999999-07:00", "2006-01-02 15:04:05-07:00", "2006-01-02 15:04:05.999999999", "2006-01-02 15:04:05.999999", "2006-01-02 15:04:05"} {
		if t, err := time.Parse(layout, value); err == nil {
			return t
		}
	}
	return time.Time{}
}
func (r *Repository) Create(ctx context.Context, x Project) (*Project, error) {
	now := time.Now()
	res, err := r.db.ExecContext(ctx, `INSERT INTO projects(name,description,repo,webhook_secret,webhook_branch,created_at,updated_at) VALUES(?,?,?,?,?,?,?)`, x.Name, x.Description, x.Repo, x.WebhookSecret, x.WebhookBranch, now, now)
	if err != nil {
		return nil, err
	}
	x.ID, err = res.LastInsertId()
	if err != nil {
		return nil, err
	}
	x.CreatedAt, x.UpdatedAt = now, now
	return &x, nil
}
func (r *Repository) Get(ctx context.Context, id int64) (*Project, error) {
	return scanProject(r.db.QueryRowContext(ctx, `SELECT `+projectColumns+` FROM projects WHERE id=?`, id))
}
func (r *Repository) List(ctx context.Context) ([]Project, error) {
	rows, err := r.db.QueryContext(ctx, `SELECT `+projectColumns+` FROM projects ORDER BY created_at DESC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := []Project{}
	for rows.Next() {
		x, err := scanProject(rows)
		if err != nil {
			return nil, err
		}
		items = append(items, *x)
	}
	return items, rows.Err()
}
func (r *Repository) Update(ctx context.Context, x Project) error {
	_, err := r.db.ExecContext(ctx, `UPDATE projects SET name=?,description=?,repo=?,webhook_branch=?,webhook_secret=?,updated_at=? WHERE id=?`, x.Name, x.Description, x.Repo, x.WebhookBranch, x.WebhookSecret, time.Now(), x.ID)
	return err
}
func (r *Repository) Delete(ctx context.Context, id int64) error {
	_, err := r.db.ExecContext(ctx, `DELETE FROM projects WHERE id=?`, id)
	return err
}

func (r *Repository) Migrate(ctx context.Context) error {
	_, err := r.db.ExecContext(ctx, `UPDATE projects SET repo='' WHERE repo IS NULL; UPDATE projects SET webhook_branch='main' WHERE webhook_branch IS NULL OR webhook_branch=''; UPDATE projects SET webhook_secret=lower(hex(randomblob(32))) WHERE webhook_secret IS NULL OR webhook_secret='';`)
	return err
}

const deploymentColumns = `id,project_id,status,docker_image,dockerfile,container_name,container_id,command,"commit",commit_msg,author,triggered_by,started_at,completed_at,created_at,updated_at`

func scanDeployment(row interface{ Scan(...any) error }) (*Deployment, error) {
	var x Deployment
	var started, completed, created, updated sql.NullString
	if err := row.Scan(&x.ID, &x.ProjectID, &x.Status, &x.DockerImage, &x.Dockerfile, &x.ContainerName, &x.ContainerID, &x.Command, &x.Commit, &x.CommitMsg, &x.Author, &x.TriggeredBy, &started, &completed, &created, &updated); err != nil {
		return nil, err
	}
	if started.Valid {
		t := parseTime(started.String)
		x.StartedAt = &t
	}
	if completed.Valid {
		t := parseTime(completed.String)
		x.CompletedAt = &t
	}
	if created.Valid {
		x.CreatedAt = parseTime(created.String)
	}
	if updated.Valid {
		x.UpdatedAt = parseTime(updated.String)
	}
	return &x, nil
}
func (r *Repository) CreateDeployment(ctx context.Context, x Deployment) (*Deployment, error) {
	now := time.Now()
	res, err := r.db.ExecContext(ctx, `INSERT INTO deployments(project_id,status,docker_image,dockerfile,container_name,container_id,command,"commit",commit_msg,author,triggered_by,created_at,updated_at) VALUES(?,'pending',?,?,?,'',?,?,?,?,?,?,?)`, x.ProjectID, x.DockerImage, x.Dockerfile, x.ContainerName, x.Command, x.Commit, x.CommitMsg, x.Author, x.TriggeredBy, now, now)
	if err != nil {
		return nil, err
	}
	x.ID, err = res.LastInsertId()
	if err != nil {
		return nil, err
	}
	x.Status = DeploymentPending
	x.CreatedAt, x.UpdatedAt = now, now
	return &x, nil
}
func (r *Repository) CreateGitHubDeployment(ctx context.Context, x Deployment, deliveryID string) (*Deployment, error) {
	now := time.Now()
	res, err := r.db.ExecContext(ctx, `INSERT INTO deployments(project_id,status,docker_image,dockerfile,container_name,container_id,command,"commit",commit_msg,author,triggered_by,github_delivery_id,created_at,updated_at) VALUES(?,'pending','','','','','',?,?,?,'GitHub push',?,?,?)`, x.ProjectID, x.Commit, x.CommitMsg, x.Author, deliveryID, now, now)
	if err != nil {
		return nil, err
	}
	x.ID, err = res.LastInsertId()
	if err != nil {
		return nil, err
	}
	x.Status = DeploymentPending
	x.TriggeredBy = "GitHub push"
	x.CreatedAt, x.UpdatedAt = now, now
	return &x, nil
}
func (r *Repository) GetDeployment(ctx context.Context, id int64) (*Deployment, error) {
	return scanDeployment(r.db.QueryRowContext(ctx, `SELECT `+deploymentColumns+` FROM deployments WHERE id=?`, id))
}
func (r *Repository) DeploymentByDelivery(ctx context.Context, delivery string) (*Deployment, error) {
	return scanDeployment(r.db.QueryRowContext(ctx, `SELECT `+deploymentColumns+` FROM deployments WHERE github_delivery_id=?`, delivery))
}
func (r *Repository) DeploymentsByProject(ctx context.Context, projectID int64) ([]Deployment, error) {
	rows, err := r.db.QueryContext(ctx, `SELECT `+deploymentColumns+` FROM deployments WHERE project_id=? ORDER BY created_at DESC`, projectID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := []Deployment{}
	for rows.Next() {
		x, err := scanDeployment(rows)
		if err != nil {
			return nil, err
		}
		items = append(items, *x)
	}
	return items, rows.Err()
}
func (r *Repository) LatestDeployment(ctx context.Context, projectID int64, successful bool) (*Deployment, error) {
	query := `SELECT ` + deploymentColumns + ` FROM deployments WHERE project_id=? ORDER BY created_at DESC LIMIT 1`
	if successful {
		query = `SELECT ` + deploymentColumns + ` FROM deployments WHERE project_id=? AND status='completed' AND container_id<>'' ORDER BY created_at DESC LIMIT 1`
	}
	return scanDeployment(r.db.QueryRowContext(ctx, query, projectID))
}
func (r *Repository) ActiveDeployment(ctx context.Context, projectID int64) (*Deployment, error) {
	return scanDeployment(r.db.QueryRowContext(ctx, `SELECT `+deploymentColumns+` FROM deployments WHERE project_id=? AND status IN ('pending','running') ORDER BY created_at DESC LIMIT 1`, projectID))
}
func (r *Repository) UpdateDeploymentStatus(ctx context.Context, id int64, status DeploymentStatus) error {
	now := time.Now()
	query := `UPDATE deployments SET status=?,updated_at=?`
	args := []any{status, now}
	if status == DeploymentRunning {
		query += `,started_at=?`
		args = append(args, now)
	}
	if status == DeploymentCompleted || status == DeploymentFailed || status == DeploymentCancelled {
		query += `,completed_at=?`
		args = append(args, now)
	}
	query += ` WHERE id=?`
	args = append(args, id)
	_, err := r.db.ExecContext(ctx, query, args...)
	return err
}
func (r *Repository) UpdateDeploymentContainer(ctx context.Context, id int64, containerID string) error {
	_, err := r.db.ExecContext(ctx, `UPDATE deployments SET container_id=?,updated_at=? WHERE id=?`, containerID, time.Now(), id)
	return err
}
func (r *Repository) UpdateDeploymentSource(ctx context.Context, id int64, commit, msg, author string) error {
	_, err := r.db.ExecContext(ctx, `UPDATE deployments SET "commit"=?,commit_msg=?,author=?,updated_at=? WHERE id=?`, commit, msg, author, time.Now(), id)
	return err
}
func (r *Repository) UpdateDeploymentArtifact(ctx context.Context, id int64, image, name string) error {
	_, err := r.db.ExecContext(ctx, `UPDATE deployments SET docker_image=?,container_name=?,updated_at=? WHERE id=?`, image, name, time.Now(), id)
	return err
}
func (r *Repository) AddDeploymentLog(ctx context.Context, id int64, message string) error {
	_, err := r.db.ExecContext(ctx, `INSERT INTO deployment_logs(deployment_id,message,created_at) VALUES(?,?,?)`, id, message, time.Now())
	return err
}
func (r *Repository) DeploymentLogs(ctx context.Context, id int64) ([]DeploymentLog, error) {
	rows, err := r.db.QueryContext(ctx, `SELECT id,deployment_id,message,created_at FROM deployment_logs WHERE deployment_id=? ORDER BY id`, id)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := []DeploymentLog{}
	for rows.Next() {
		var x DeploymentLog
		var created sql.NullString
		if err := rows.Scan(&x.ID, &x.DeploymentID, &x.Message, &created); err != nil {
			return nil, err
		}
		if created.Valid {
			x.CreatedAt = parseTime(created.String)
		}
		items = append(items, x)
	}
	return items, rows.Err()
}
func (r *Repository) RecoverInterruptedDeployments(ctx context.Context) error {
	now := time.Now()
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err = tx.ExecContext(ctx, `INSERT INTO deployment_logs(deployment_id,message,created_at) SELECT id,?,? FROM deployments WHERE status IN ('pending','running')`, fmt.Sprintf("[%s] Deployment interrupted by server restart", now.Format("2006-01-02 15:04:05")), now); err != nil {
		return err
	}
	if _, err = tx.ExecContext(ctx, `UPDATE deployments SET status='failed',completed_at=?,updated_at=? WHERE status IN ('pending','running')`, now, now); err != nil {
		return err
	}
	return tx.Commit()
}

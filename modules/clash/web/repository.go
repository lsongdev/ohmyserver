package web

import (
	"context"
	"database/sql"
	"time"
)

type Server struct {
	ID          int64     `json:"id"`
	Name        string    `json:"name"`
	APIURL      string    `json:"api_url"`
	Secret      string    `json:"-"`
	Status      string    `json:"status"`
	Description string    `json:"description"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
}
type Repository struct{ db *sql.DB }

func NewRepository(db *sql.DB) *Repository { return &Repository{db: db} }
func (r *Repository) Initialize(ctx context.Context) error {
	_, err := r.db.ExecContext(ctx, `CREATE TABLE IF NOT EXISTS clash_servers(id INTEGER PRIMARY KEY AUTOINCREMENT,name TEXT NOT NULL UNIQUE,api_url TEXT NOT NULL,secret TEXT NOT NULL,status TEXT NOT NULL DEFAULT 'active',description TEXT DEFAULT '',created_at DATETIME DEFAULT CURRENT_TIMESTAMP,updated_at DATETIME DEFAULT CURRENT_TIMESTAMP); CREATE INDEX IF NOT EXISTS idx_clash_servers_name ON clash_servers(name);`)
	return err
}
func (r *Repository) Create(ctx context.Context, v *Server) error {
	now := time.Now()
	res, err := r.db.ExecContext(ctx, `INSERT INTO clash_servers(name,api_url,secret,status,description,created_at,updated_at) VALUES(?,?,?,'active',?,?,?)`, v.Name, v.APIURL, v.Secret, v.Description, now, now)
	if err != nil {
		return err
	}
	v.ID, err = res.LastInsertId()
	v.Status = "active"
	v.CreatedAt = now
	v.UpdatedAt = now
	return err
}
func (r *Repository) List(ctx context.Context) ([]Server, error) {
	rows, err := r.db.QueryContext(ctx, `SELECT id,name,api_url,secret,status,description,created_at,updated_at FROM clash_servers ORDER BY created_at DESC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := make([]Server, 0)
	for rows.Next() {
		v, e := scan(rows)
		if e != nil {
			return nil, e
		}
		items = append(items, *v)
	}
	return items, rows.Err()
}
func (r *Repository) Get(ctx context.Context, id int64) (*Server, error) {
	return scan(r.db.QueryRowContext(ctx, `SELECT id,name,api_url,secret,status,description,created_at,updated_at FROM clash_servers WHERE id=?`, id))
}
func (r *Repository) Update(ctx context.Context, v *Server) error {
	now := time.Now()
	_, err := r.db.ExecContext(ctx, `UPDATE clash_servers SET name=?,api_url=?,secret=?,description=?,updated_at=? WHERE id=?`, v.Name, v.APIURL, v.Secret, v.Description, now, v.ID)
	if err == nil {
		v.UpdatedAt = now
	}
	return err
}
func (r *Repository) Delete(ctx context.Context, id int64) error {
	_, err := r.db.ExecContext(ctx, `DELETE FROM clash_servers WHERE id=?`, id)
	return err
}
func scan(row interface{ Scan(...any) error }) (*Server, error) {
	var v Server
	var created, updated string
	err := row.Scan(&v.ID, &v.Name, &v.APIURL, &v.Secret, &v.Status, &v.Description, &created, &updated)
	if err != nil {
		return nil, err
	}
	v.CreatedAt = parseTime(created)
	v.UpdatedAt = parseTime(updated)
	return &v, nil
}
func parseTime(v string) time.Time {
	for _, layout := range []string{time.RFC3339Nano, "2006-01-02 15:04:05", "2006-01-02 15:04:05-07:00"} {
		if t, e := time.Parse(layout, v); e == nil {
			return t
		}
	}
	return time.Time{}
}

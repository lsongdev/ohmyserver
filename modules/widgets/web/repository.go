package web

import (
	"context"
	"database/sql"
	"time"
)

type Repository struct{ db *sql.DB }

func NewRepository(db *sql.DB) *Repository { return &Repository{db: db} }
func (r *Repository) Initialize(ctx context.Context) error {
	_, err := r.db.ExecContext(ctx, `CREATE TABLE IF NOT EXISTS widgets(id INTEGER PRIMARY KEY AUTOINCREMENT,name TEXT NOT NULL,type TEXT NOT NULL,config TEXT DEFAULT '',sort_order INTEGER DEFAULT 0,enabled INTEGER DEFAULT 1,created_at DATETIME DEFAULT CURRENT_TIMESTAMP,updated_at DATETIME DEFAULT CURRENT_TIMESTAMP);CREATE INDEX IF NOT EXISTS idx_widgets_enabled ON widgets(enabled);CREATE INDEX IF NOT EXISTS idx_widgets_sort_order ON widgets(sort_order);`)
	return err
}

const columns = `id,name,type,config,sort_order,enabled,created_at,updated_at`

func scan(row interface{ Scan(...any) error }) (*Widget, error) {
	var x Widget
	var created, updated string
	err := row.Scan(&x.ID, &x.Name, &x.Type, &x.Config, &x.SortOrder, &x.Enabled, &created, &updated)
	if err != nil {
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
func (r *Repository) List(ctx context.Context, enabledOnly bool) ([]Widget, error) {
	query := `SELECT ` + columns + ` FROM widgets ORDER BY sort_order ASC,name ASC`
	if enabledOnly {
		query = `SELECT ` + columns + ` FROM widgets WHERE enabled=1 ORDER BY sort_order ASC,name ASC`
	}
	rows, err := r.db.QueryContext(ctx, query)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := []Widget{}
	for rows.Next() {
		x, err := scan(rows)
		if err != nil {
			return nil, err
		}
		items = append(items, *x)
	}
	return items, rows.Err()
}
func (r *Repository) Get(ctx context.Context, id int64) (*Widget, error) {
	return scan(r.db.QueryRowContext(ctx, `SELECT `+columns+` FROM widgets WHERE id=?`, id))
}
func (r *Repository) Create(ctx context.Context, x *Widget) error {
	now := time.Now()
	res, err := r.db.ExecContext(ctx, `INSERT INTO widgets(name,type,config,sort_order,enabled,created_at,updated_at) VALUES(?,?,?,?,?,?,?)`, x.Name, x.Type, x.Config, x.SortOrder, x.Enabled, now, now)
	if err != nil {
		return err
	}
	x.ID, err = res.LastInsertId()
	if err == nil {
		x.CreatedAt, x.UpdatedAt = now, now
	}
	return err
}
func (r *Repository) Update(ctx context.Context, x Widget) error {
	now := time.Now()
	_, err := r.db.ExecContext(ctx, `UPDATE widgets SET name=?,type=?,config=?,sort_order=?,enabled=?,updated_at=? WHERE id=?`, x.Name, x.Type, x.Config, x.SortOrder, x.Enabled, now, x.ID)
	return err
}
func (r *Repository) Delete(ctx context.Context, id int64) error {
	_, err := r.db.ExecContext(ctx, `DELETE FROM widgets WHERE id=?`, id)
	return err
}

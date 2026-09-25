package web

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	settings "github.com/lsongdev/ohmyserver/modules/settings"
)

type Setting = settings.Setting

type Repository struct{ db *sql.DB }

func NewRepository(db *sql.DB) *Repository { return &Repository{db: db} }

func (r *Repository) Initialize(ctx context.Context) error {
	_, err := r.db.ExecContext(ctx, `CREATE TABLE IF NOT EXISTS settings (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		name TEXT NOT NULL UNIQUE,
		type TEXT NOT NULL DEFAULT 'string',
		value TEXT NOT NULL,
		description TEXT DEFAULT '',
		created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
		updated_at DATETIME DEFAULT CURRENT_TIMESTAMP
	);
	CREATE INDEX IF NOT EXISTS idx_settings_name ON settings(name);
	CREATE INDEX IF NOT EXISTS idx_settings_type ON settings(type);`)
	return err
}

func (r *Repository) Create(ctx context.Context, name, kind, value, description string) (*Setting, error) {
	if !validType(kind) {
		return nil, fmt.Errorf("invalid setting type: %s", kind)
	}
	now := time.Now()
	result, err := r.db.ExecContext(ctx, `INSERT INTO settings (name,type,value,description,created_at,updated_at) VALUES (?,?,?,?,?,?)`, name, kind, value, description, now, now)
	if err != nil {
		return nil, err
	}
	id, err := result.LastInsertId()
	if err != nil {
		return nil, err
	}
	return &Setting{ID: id, Name: name, Type: kind, Value: value, Description: description, CreatedAt: now, UpdatedAt: now}, nil
}

func (r *Repository) List(ctx context.Context) ([]Setting, error) {
	rows, err := r.db.QueryContext(ctx, `SELECT id,name,type,value,description,created_at,updated_at FROM settings ORDER BY name ASC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := make([]Setting, 0)
	for rows.Next() {
		var item Setting
		var created, updated string
		if err := rows.Scan(&item.ID, &item.Name, &item.Type, &item.Value, &item.Description, &created, &updated); err != nil {
			return nil, err
		}
		item.CreatedAt, item.UpdatedAt = parseTime(created), parseTime(updated)
		items = append(items, item)
	}
	return items, rows.Err()
}

func (r *Repository) Get(ctx context.Context, id int64) (*Setting, error) {
	var item Setting
	var created, updated string
	err := r.db.QueryRowContext(ctx, `SELECT id,name,type,value,description,created_at,updated_at FROM settings WHERE id=?`, id).Scan(&item.ID, &item.Name, &item.Type, &item.Value, &item.Description, &created, &updated)
	if err != nil {
		return nil, err
	}
	item.CreatedAt, item.UpdatedAt = parseTime(created), parseTime(updated)
	return &item, nil
}

func (r *Repository) GetByName(ctx context.Context, name string) (*Setting, error) {
	var item Setting
	var created, updated string
	err := r.db.QueryRowContext(ctx, `SELECT id,name,type,value,description,created_at,updated_at FROM settings WHERE name=?`, name).Scan(&item.ID, &item.Name, &item.Type, &item.Value, &item.Description, &created, &updated)
	if err != nil {
		return nil, err
	}
	item.CreatedAt, item.UpdatedAt = parseTime(created), parseTime(updated)
	return &item, nil
}

func (r *Repository) Update(ctx context.Context, id int64, value string) error {
	result, err := r.db.ExecContext(ctx, `UPDATE settings SET value=?,updated_at=? WHERE id=?`, value, time.Now(), id)
	if err != nil {
		return err
	}
	count, err := result.RowsAffected()
	if err == nil && count == 0 {
		return sql.ErrNoRows
	}
	return err
}

func (r *Repository) Delete(ctx context.Context, id int64) error {
	result, err := r.db.ExecContext(ctx, `DELETE FROM settings WHERE id=?`, id)
	if err != nil {
		return err
	}
	count, err := result.RowsAffected()
	if err == nil && count == 0 {
		return sql.ErrNoRows
	}
	return err
}

func validType(kind string) bool {
	return kind == "string" || kind == "number" || kind == "boolean" || kind == "json"
}

func parseTime(value string) time.Time {
	for _, layout := range []string{time.RFC3339Nano, "2006-01-02 15:04:05", "2006-01-02 15:04:05-07:00"} {
		if parsed, err := time.Parse(layout, value); err == nil {
			return parsed
		}
	}
	return time.Time{}
}

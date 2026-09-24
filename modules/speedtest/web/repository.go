package web

import (
	"context"
	"database/sql"
	"time"
)

type Repository struct{ db *sql.DB }

func NewRepository(db *sql.DB) *Repository { return &Repository{db: db} }

func (r *Repository) Initialize(ctx context.Context) error {
	_, err := r.db.ExecContext(ctx, `CREATE TABLE IF NOT EXISTS speedtest_results (
		id INTEGER PRIMARY KEY AUTOINCREMENT, type TEXT NOT NULL, server_id TEXT NOT NULL,
		server_name TEXT NOT NULL, server_host TEXT NOT NULL, download REAL NOT NULL,
		upload REAL NOT NULL, ping REAL NOT NULL, jitter REAL NOT NULL DEFAULT 0,
		ip TEXT NOT NULL, isp TEXT DEFAULT '', country TEXT DEFAULT '',
		created_at DATETIME DEFAULT CURRENT_TIMESTAMP, updated_at DATETIME DEFAULT CURRENT_TIMESTAMP
	); CREATE INDEX IF NOT EXISTS idx_speedtest_created ON speedtest_results(created_at);`)
	return err
}

const resultColumns = `id,type,server_id,server_name,server_host,download,upload,ping,jitter,ip,isp,country,created_at,updated_at`

func scanResult(row interface{ Scan(...any) error }) (*Result, error) {
	var item Result
	var created, updated string
	err := row.Scan(&item.ID, &item.Type, &item.ServerID, &item.ServerName, &item.ServerHost, &item.Download, &item.Upload, &item.Ping, &item.Jitter, &item.IP, &item.ISP, &item.Country, &created, &updated)
	if err != nil {
		return nil, err
	}
	item.CreatedAt, item.UpdatedAt = parseTime(created), parseTime(updated)
	return &item, nil
}

func (r *Repository) Create(ctx context.Context, item *Result) error {
	now := time.Now()
	result, err := r.db.ExecContext(ctx, `INSERT INTO speedtest_results (type,server_id,server_name,server_host,download,upload,ping,jitter,ip,isp,country,created_at,updated_at) VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?)`, item.Type, item.ServerID, item.ServerName, item.ServerHost, item.Download, item.Upload, item.Ping, item.Jitter, item.IP, item.ISP, item.Country, now, now)
	if err != nil {
		return err
	}
	item.ID, err = result.LastInsertId()
	item.CreatedAt, item.UpdatedAt = now, now
	return err
}

func (r *Repository) List(ctx context.Context) ([]Result, error) {
	rows, err := r.db.QueryContext(ctx, `SELECT `+resultColumns+` FROM speedtest_results ORDER BY created_at DESC LIMIT 100`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := make([]Result, 0)
	for rows.Next() {
		item, err := scanResult(rows)
		if err != nil {
			return nil, err
		}
		items = append(items, *item)
	}
	return items, rows.Err()
}

func (r *Repository) Get(ctx context.Context, id int64) (*Result, error) {
	return scanResult(r.db.QueryRowContext(ctx, `SELECT `+resultColumns+` FROM speedtest_results WHERE id=?`, id))
}

func (r *Repository) Delete(ctx context.Context, id int64) error {
	_, err := r.db.ExecContext(ctx, `DELETE FROM speedtest_results WHERE id=?`, id)
	return err
}

func parseTime(value string) time.Time {
	for _, layout := range []string{time.RFC3339Nano, "2006-01-02 15:04:05", "2006-01-02 15:04:05-07:00"} {
		if parsed, err := time.Parse(layout, value); err == nil {
			return parsed
		}
	}
	return time.Time{}
}

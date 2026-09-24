// Package ntp owns the configuration persisted by the NTP service.
package web

import (
	"context"
	"database/sql"
	"time"

	ntp "github.com/lsongdev/ohmyserver/modules/ntp"
)

type Settings = ntp.Settings
type Repository struct{ db *sql.DB }

func NewRepository(db *sql.DB) *Repository { return &Repository{db: db} }
func (r *Repository) Initialize(ctx context.Context) error {
	_, err := r.db.ExecContext(ctx, `CREATE TABLE IF NOT EXISTS ntp_settings(id INTEGER PRIMARY KEY CHECK(id=1),enabled INTEGER NOT NULL DEFAULT 0,listen_addr TEXT NOT NULL DEFAULT ':123',stratum INTEGER NOT NULL DEFAULT 2,reference_id TEXT NOT NULL DEFAULT 'LOCL',updated_at DATETIME DEFAULT CURRENT_TIMESTAMP);INSERT OR IGNORE INTO ntp_settings(id) VALUES(1);`)
	return err
}
func (r *Repository) Get(ctx context.Context) (*Settings, error) {
	var x Settings
	var enabled int
	var updated string
	err := r.db.QueryRowContext(ctx, `SELECT enabled,listen_addr,stratum,reference_id,updated_at FROM ntp_settings WHERE id=1`).Scan(&enabled, &x.ListenAddr, &x.Stratum, &x.ReferenceID, &updated)
	if err != nil {
		return nil, err
	}
	x.Enabled = enabled != 0
	x.UpdatedAt = parseTime(updated)
	return &x, nil
}
func (r *Repository) Update(ctx context.Context, x Settings) error {
	_, err := r.db.ExecContext(ctx, `UPDATE ntp_settings SET enabled=?,listen_addr=?,stratum=?,reference_id=?,updated_at=? WHERE id=1`, x.Enabled, x.ListenAddr, x.Stratum, x.ReferenceID, time.Now())
	return err
}
func parseTime(v string) time.Time {
	for _, f := range []string{time.RFC3339Nano, "2006-01-02 15:04:05.999999999-07:00", "2006-01-02 15:04:05.999999999", "2006-01-02 15:04:05-07:00", "2006-01-02 15:04:05"} {
		if t, e := time.Parse(f, v); e == nil {
			return t
		}
	}
	return time.Time{}
}

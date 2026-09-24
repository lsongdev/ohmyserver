package web

import (
	"context"
	"database/sql"
	"time"
)

type Repository struct{ db *sql.DB }

func NewRepository(db *sql.DB) *Repository { return &Repository{db: db} }

func (r *Repository) Initialize(ctx context.Context) error {
	_, err := r.db.ExecContext(ctx, `CREATE TABLE IF NOT EXISTS monitors (
		id INTEGER PRIMARY KEY AUTOINCREMENT, name TEXT NOT NULL, type TEXT NOT NULL DEFAULT 'http', target TEXT NOT NULL,
		port INTEGER DEFAULT 0, interval INTEGER NOT NULL DEFAULT 60, timeout INTEGER NOT NULL DEFAULT 10,
		expected_status INTEGER DEFAULT 200, enabled INTEGER DEFAULT 1,
		created_at DATETIME DEFAULT CURRENT_TIMESTAMP, updated_at DATETIME DEFAULT CURRENT_TIMESTAMP);
		CREATE INDEX IF NOT EXISTS idx_monitors_enabled ON monitors(enabled);
		CREATE TABLE IF NOT EXISTS monitor_checks (
		id INTEGER PRIMARY KEY AUTOINCREMENT, monitor_id INTEGER NOT NULL, status TEXT NOT NULL,
		response_time INTEGER DEFAULT 0, status_code INTEGER DEFAULT 0, message TEXT DEFAULT '',
		checked_at DATETIME DEFAULT CURRENT_TIMESTAMP, FOREIGN KEY(monitor_id) REFERENCES monitors(id) ON DELETE CASCADE);
		CREATE INDEX IF NOT EXISTS idx_monitor_checks_monitor_id ON monitor_checks(monitor_id);
		CREATE INDEX IF NOT EXISTS idx_monitor_checks_checked_at ON monitor_checks(checked_at);`)
	return err
}

const monitorColumns = `id,name,type,target,port,interval,timeout,expected_status,enabled,created_at,updated_at`

func scanMonitor(row interface{ Scan(...any) error }) (*Monitor, error) {
	var m Monitor
	var enabled int
	var created, updated string
	err := row.Scan(&m.ID, &m.Name, &m.Type, &m.Target, &m.Port, &m.Interval, &m.Timeout, &m.ExpectedStatus, &enabled, &created, &updated)
	if err != nil {
		return nil, err
	}
	m.Enabled = enabled != 0
	m.CreatedAt = parseTime(created)
	m.UpdatedAt = parseTime(updated)
	return &m, nil
}
func parseTime(v string) time.Time {
	for _, layout := range []string{
		time.RFC3339Nano,
		"2006-01-02 15:04:05.999999999-07:00", "2006-01-02 15:04:05.999999-07:00", "2006-01-02 15:04:05-07:00",
		"2006-01-02 15:04:05.999999999", "2006-01-02 15:04:05.999999", "2006-01-02 15:04:05",
		"2006-01-02T15:04:05.999999999-07:00", "2006-01-02T15:04:05-07:00",
	} {
		if t, err := time.Parse(layout, v); err == nil {
			return t
		}
	}
	return time.Time{}
}
func (r *Repository) Monitors(ctx context.Context, enabledOnly bool) ([]Monitor, error) {
	query := `SELECT ` + monitorColumns + ` FROM monitors ORDER BY created_at DESC`
	if enabledOnly {
		query = `SELECT ` + monitorColumns + ` FROM monitors WHERE enabled=1 ORDER BY created_at DESC`
	}
	rows, err := r.db.QueryContext(ctx, query)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := []Monitor{}
	for rows.Next() {
		m, err := scanMonitor(rows)
		if err != nil {
			return nil, err
		}
		items = append(items, *m)
	}
	return items, rows.Err()
}
func (r *Repository) Monitor(ctx context.Context, id int64) (*Monitor, error) {
	return scanMonitor(r.db.QueryRowContext(ctx, `SELECT `+monitorColumns+` FROM monitors WHERE id=?`, id))
}
func (r *Repository) SaveMonitor(ctx context.Context, m *Monitor) error {
	now := time.Now()
	if m.ID == 0 {
		res, err := r.db.ExecContext(ctx, `INSERT INTO monitors(name,type,target,port,interval,timeout,expected_status,enabled,created_at,updated_at) VALUES(?,?,?,?,?,?,?,?,?,?)`, m.Name, m.Type, m.Target, m.Port, m.Interval, m.Timeout, m.ExpectedStatus, m.Enabled, now, now)
		if err != nil {
			return err
		}
		m.ID, err = res.LastInsertId()
		if err != nil {
			return err
		}
		m.CreatedAt, m.UpdatedAt = now, now
		return nil
	}
	_, err := r.db.ExecContext(ctx, `UPDATE monitors SET name=?,type=?,target=?,port=?,interval=?,timeout=?,expected_status=?,enabled=?,updated_at=? WHERE id=?`, m.Name, m.Type, m.Target, m.Port, m.Interval, m.Timeout, m.ExpectedStatus, m.Enabled, now, m.ID)
	m.UpdatedAt = now
	return err
}
func (r *Repository) DeleteMonitor(ctx context.Context, id int64) error {
	_, err := r.db.ExecContext(ctx, `DELETE FROM monitors WHERE id=?`, id)
	return err
}
func (r *Repository) SetEnabled(ctx context.Context, id int64, enabled bool) error {
	_, err := r.db.ExecContext(ctx, `UPDATE monitors SET enabled=?,updated_at=? WHERE id=?`, enabled, time.Now(), id)
	return err
}
func (r *Repository) InsertCheck(ctx context.Context, c *Check) error {
	res, err := r.db.ExecContext(ctx, `INSERT INTO monitor_checks(monitor_id,status,response_time,status_code,message,checked_at) VALUES(?,?,?,?,?,?)`, c.MonitorID, c.Status, c.ResponseTime, c.StatusCode, c.Message, c.CheckedAt)
	if err != nil {
		return err
	}
	c.ID, err = res.LastInsertId()
	return err
}
func scanCheck(row interface{ Scan(...any) error }) (*Check, error) {
	var c Check
	var checked string
	err := row.Scan(&c.ID, &c.MonitorID, &c.Status, &c.ResponseTime, &c.StatusCode, &c.Message, &checked)
	if err != nil {
		return nil, err
	}
	c.CheckedAt = parseTime(checked)
	return &c, nil
}

const checkColumns = `id,monitor_id,status,response_time,status_code,message,checked_at`

func (r *Repository) LatestCheck(ctx context.Context, id int64) (*Check, error) {
	return scanCheck(r.db.QueryRowContext(ctx, `SELECT `+checkColumns+` FROM monitor_checks WHERE monitor_id=? ORDER BY checked_at DESC LIMIT 1`, id))
}
func (r *Repository) Checks(ctx context.Context, id int64, limit int) ([]Check, error) {
	rows, err := r.db.QueryContext(ctx, `SELECT `+checkColumns+` FROM monitor_checks WHERE monitor_id=? ORDER BY checked_at DESC LIMIT ?`, id, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := []Check{}
	for rows.Next() {
		c, err := scanCheck(rows)
		if err != nil {
			return nil, err
		}
		items = append(items, *c)
	}
	return items, rows.Err()
}
func (r *Repository) Uptime(ctx context.Context, id int64, hours int) (float64, error) {
	var total, up int64
	err := r.db.QueryRowContext(ctx, `SELECT COUNT(*),COALESCE(SUM(CASE WHEN status='up' THEN 1 ELSE 0 END),0) FROM monitor_checks WHERE monitor_id=? AND checked_at>=?`, id, time.Now().Add(-time.Duration(hours)*time.Hour)).Scan(&total, &up)
	if err != nil {
		return 0, err
	}
	if total == 0 {
		return 100, nil
	}
	return float64(up) / float64(total) * 100, nil
}
func (r *Repository) Cleanup(ctx context.Context, days int) error {
	_, err := r.db.ExecContext(ctx, `DELETE FROM monitor_checks WHERE checked_at<?`, time.Now().Add(-time.Duration(days)*24*time.Hour))
	return err
}

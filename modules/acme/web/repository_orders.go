package web

import (
	"context"
	"database/sql"
	acme "github.com/lsongdev/ohmyserver/modules/acme"
	"time"
)

type Authorization = acme.Authorization
type Challenge = acme.Challenge
type Identifier = acme.Identifier

var ErrIssuanceInProgress = acme.ErrIssuanceInProgress

func (r *Repository) ListIssuances(ctx context.Context) ([]Issuance, error) {
	rows, err := r.db.QueryContext(ctx, `SELECT id, certificate_id, account_id, order_id, domains, challenge_type, dns_target, status, step, error, created_at, updated_at FROM acme_issuances ORDER BY created_at DESC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := make([]Issuance, 0)
	for rows.Next() {
		var item Issuance
		var created, updated string
		if err := rows.Scan(&item.ID, &item.CertificateID, &item.AccountID, &item.OrderID, &item.Domains, &item.ChallengeType, &item.DNSTarget, &item.Status, &item.Step, &item.Error, &created, &updated); err != nil {
			return nil, err
		}
		item.CreatedAt, item.UpdatedAt = parseTime(created), parseTime(updated)
		items = append(items, item)
	}
	return items, rows.Err()
}

func (r *Repository) InitializeIssuances(ctx context.Context) error {
	_, err := r.db.ExecContext(ctx, `
		CREATE TABLE IF NOT EXISTS acme_issuances (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			certificate_id INTEGER NOT NULL,
			account_id INTEGER NOT NULL,
			order_id INTEGER NOT NULL DEFAULT 0,
			domains TEXT NOT NULL,
			challenge_type TEXT NOT NULL,
			dns_target TEXT NOT NULL DEFAULT '',
			status TEXT NOT NULL DEFAULT 'queued',
			step TEXT NOT NULL DEFAULT 'queued',
			error TEXT NOT NULL DEFAULT '',
			created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
			updated_at DATETIME DEFAULT CURRENT_TIMESTAMP
		);
		CREATE INDEX IF NOT EXISTS idx_acme_issuances_certificate ON acme_issuances(certificate_id);
		CREATE INDEX IF NOT EXISTS idx_acme_issuances_status ON acme_issuances(status);
	`)
	return err
}

// RecoverInterruptedIssuances marks work that was in flight during the prior
// process lifetime. This is deliberately separate from idempotent schema setup.
func (r *Repository) RecoverInterruptedIssuances(ctx context.Context) error {
	_, err := r.db.ExecContext(ctx, `UPDATE acme_issuances SET status='failed', step='interrupted', error='OhMyServer stopped before this issuance completed', updated_at=CURRENT_TIMESTAMP WHERE status IN ('queued', 'running')`)
	return err
}

func (r *Repository) CreateIssuance(ctx context.Context, item *Issuance) error {
	now := time.Now()
	result, err := r.db.ExecContext(ctx, `INSERT INTO acme_issuances (certificate_id, account_id, domains, challenge_type, dns_target, status, step, created_at, updated_at) VALUES (?, ?, ?, ?, ?, 'queued', 'queued', ?, ?)`, item.CertificateID, item.AccountID, item.Domains, item.ChallengeType, item.DNSTarget, now, now)
	if err != nil {
		return err
	}
	item.ID, err = result.LastInsertId()
	item.Status, item.Step, item.CreatedAt, item.UpdatedAt = "queued", "queued", now, now
	return err
}

func (r *Repository) CreateRenewalIssuance(ctx context.Context, item *Issuance) error {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	var active int
	if err := tx.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM acme_issuances WHERE certificate_id=? AND status IN ('queued','running'))`, item.CertificateID).Scan(&active); err != nil {
		return err
	}
	if active != 0 {
		return ErrIssuanceInProgress
	}
	now := time.Now()
	result, err := tx.ExecContext(ctx, `INSERT INTO acme_issuances (certificate_id, account_id, domains, challenge_type, dns_target, status, step, created_at, updated_at) VALUES (?, ?, ?, ?, ?, 'queued', 'queued', ?, ?)`, item.CertificateID, item.AccountID, item.Domains, item.ChallengeType, item.DNSTarget, now, now)
	if err != nil {
		return err
	}
	item.ID, err = result.LastInsertId()
	if err != nil {
		return err
	}
	if err := tx.Commit(); err != nil {
		return err
	}
	item.Status, item.Step, item.CreatedAt, item.UpdatedAt = "queued", "queued", now, now
	return nil
}

func (r *Repository) LatestSuccessfulIssuance(ctx context.Context, certificateID int64) (*Issuance, error) {
	var item Issuance
	var created, updated string
	err := r.db.QueryRowContext(ctx, `SELECT id, certificate_id, account_id, order_id, domains, challenge_type, dns_target, status, step, error, created_at, updated_at FROM acme_issuances WHERE certificate_id=? AND status='succeeded' ORDER BY updated_at DESC, id DESC LIMIT 1`, certificateID).Scan(&item.ID, &item.CertificateID, &item.AccountID, &item.OrderID, &item.Domains, &item.ChallengeType, &item.DNSTarget, &item.Status, &item.Step, &item.Error, &created, &updated)
	if err != nil {
		return nil, err
	}
	item.CreatedAt, item.UpdatedAt = parseTime(created), parseTime(updated)
	return &item, nil
}

func (r *Repository) GetIssuance(ctx context.Context, id int64) (*Issuance, error) {
	var item Issuance
	var created, updated string
	err := r.db.QueryRowContext(ctx, `SELECT id, certificate_id, account_id, order_id, domains, challenge_type, dns_target, status, step, error, created_at, updated_at FROM acme_issuances WHERE id=?`, id).Scan(&item.ID, &item.CertificateID, &item.AccountID, &item.OrderID, &item.Domains, &item.ChallengeType, &item.DNSTarget, &item.Status, &item.Step, &item.Error, &created, &updated)
	if err != nil {
		return nil, err
	}
	item.CreatedAt, item.UpdatedAt = parseTime(created), parseTime(updated)
	return &item, nil
}

func (r *Repository) UpdateIssuance(ctx context.Context, id int64, status, step string, orderID int64, issueErr error) error {
	message := ""
	if issueErr != nil {
		message = issueErr.Error()
	}
	_, err := r.db.ExecContext(ctx, `UPDATE acme_issuances SET status=?, step=?, order_id=CASE WHEN ? > 0 THEN ? ELSE order_id END, error=?, updated_at=? WHERE id=?`, status, step, orderID, orderID, message, time.Now(), id)
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

func (r *Repository) InitializeOrders(ctx context.Context) error {
	_, err := r.db.ExecContext(ctx, `
		CREATE TABLE IF NOT EXISTS acme_orders (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			account_id INTEGER NOT NULL,
			order_url TEXT NOT NULL UNIQUE,
			domains TEXT,
			created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
			updated_at DATETIME DEFAULT CURRENT_TIMESTAMP,
			FOREIGN KEY (account_id) REFERENCES acme_accounts(id) ON DELETE CASCADE
		);
		CREATE INDEX IF NOT EXISTS idx_acme_orders_account_id ON acme_orders(account_id);
	`)
	return err
}

func (r *Repository) InitializeAuthorizations(ctx context.Context) error {
	_, err := r.db.ExecContext(ctx, `
		CREATE TABLE IF NOT EXISTS acme_authorizations (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			order_id INTEGER NOT NULL,
			authz_url TEXT NOT NULL UNIQUE,
			status TEXT NOT NULL DEFAULT 'pending',
			identifier_type TEXT NOT NULL,
			identifier_value TEXT NOT NULL,
			expires DATETIME,
			created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
			FOREIGN KEY (order_id) REFERENCES acme_orders(id) ON DELETE CASCADE
		);
		CREATE INDEX IF NOT EXISTS idx_acme_authorizations_order_id ON acme_authorizations(order_id);
	`)
	return err
}

func (r *Repository) InitializeChallenges(ctx context.Context) error {
	_, err := r.db.ExecContext(ctx, `
		CREATE TABLE IF NOT EXISTS acme_challenges (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			authorization_id INTEGER NOT NULL,
			challenge_url TEXT NOT NULL UNIQUE,
			type TEXT NOT NULL,
			status TEXT NOT NULL DEFAULT 'pending',
			token TEXT NOT NULL,
			key_auth TEXT,
			validated DATETIME,
			error TEXT,
			created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
			FOREIGN KEY (authorization_id) REFERENCES acme_authorizations(id) ON DELETE CASCADE
		);
		CREATE INDEX IF NOT EXISTS idx_acme_challenges_authz_id ON acme_challenges(authorization_id);
	`)
	return err
}

func (r *Repository) CreateOrder(ctx context.Context, accountID int64, orderURL, domains string) (*Order, error) {
	now := time.Now()
	result, err := r.db.ExecContext(ctx, `INSERT INTO acme_orders (account_id, order_url, domains, created_at, updated_at) VALUES (?, ?, ?, ?, ?)`, accountID, orderURL, domains, now, now)
	if err != nil {
		return nil, err
	}
	id, err := result.LastInsertId()
	if err != nil {
		return nil, err
	}
	return &Order{ID: id, AccountID: accountID, OrderURL: orderURL, Domains: domains, CreatedAt: now.Format("2006-01-02 15:04:05"), UpdatedAt: now.Format("2006-01-02 15:04:05")}, nil
}

func (r *Repository) GetOrder(ctx context.Context, id int64) (*Order, error) {
	var item Order
	err := r.db.QueryRowContext(ctx, `SELECT id, account_id, order_url, domains, created_at, updated_at FROM acme_orders WHERE id=?`, id).Scan(&item.ID, &item.AccountID, &item.OrderURL, &item.Domains, &item.CreatedAt, &item.UpdatedAt)
	if err != nil {
		return nil, err
	}
	return &item, nil
}

func (r *Repository) ListOrders(ctx context.Context) ([]Order, error) {
	rows, err := r.db.QueryContext(ctx, `SELECT id, account_id, order_url, domains, created_at, updated_at FROM acme_orders ORDER BY created_at DESC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := make([]Order, 0)
	for rows.Next() {
		var item Order
		if err := rows.Scan(&item.ID, &item.AccountID, &item.OrderURL, &item.Domains, &item.CreatedAt, &item.UpdatedAt); err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	return items, rows.Err()
}

func (r *Repository) DeleteOrder(ctx context.Context, id int64) error {
	_, err := r.db.ExecContext(ctx, `DELETE FROM acme_orders WHERE id=?`, id)
	return err
}

func (r *Repository) CreateAuthorization(ctx context.Context, orderID int64, authzURL, status, identifierType, identifierValue string, expires time.Time) (*Authorization, error) {
	now := time.Now()
	result, err := r.db.ExecContext(ctx, `INSERT INTO acme_authorizations (order_id, authz_url, status, identifier_type, identifier_value, expires, created_at) VALUES (?, ?, ?, ?, ?, ?, ?)`, orderID, authzURL, status, identifierType, identifierValue, expires, now)
	if err != nil {
		return nil, err
	}
	id, err := result.LastInsertId()
	if err != nil {
		return nil, err
	}
	return &Authorization{ID: id, OrderID: orderID, AuthzURL: authzURL, Status: status, Identifier: Identifier{Type: identifierType, Value: identifierValue}, Expires: expires, CreatedAt: now.Format("2006-01-02 15:04:05")}, nil
}

func (r *Repository) CreateChallenge(ctx context.Context, authorizationID int64, challengeURL, challengeType, status, token, keyAuth string) (*Challenge, error) {
	now := time.Now()
	result, err := r.db.ExecContext(ctx, `INSERT INTO acme_challenges (authorization_id, challenge_url, type, status, token, key_auth, created_at) VALUES (?, ?, ?, ?, ?, ?, ?)`, authorizationID, challengeURL, challengeType, status, token, keyAuth, now)
	if err != nil {
		return nil, err
	}
	id, err := result.LastInsertId()
	if err != nil {
		return nil, err
	}
	return &Challenge{ID: id, AuthorizationID: authorizationID, ChallengeURL: challengeURL, Type: challengeType, Status: status, Token: token, KeyAuth: keyAuth, CreatedAt: now.Format("2006-01-02 15:04:05")}, nil
}

func (r *Repository) ListChallengesByAuthorization(ctx context.Context, authorizationID int64) ([]Challenge, error) {
	rows, err := r.db.QueryContext(ctx, `SELECT id, authorization_id, challenge_url, type, status, token, key_auth, validated, error, created_at FROM acme_challenges WHERE authorization_id=? ORDER BY created_at ASC`, authorizationID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := make([]Challenge, 0)
	for rows.Next() {
		var item Challenge
		var validated sql.NullString
		if err := rows.Scan(&item.ID, &item.AuthorizationID, &item.ChallengeURL, &item.Type, &item.Status, &item.Token, &item.KeyAuth, &validated, &item.Error, &item.CreatedAt); err != nil {
			return nil, err
		}
		if validated.Valid {
			item.Validated = parseACMETime(validated.String)
		}
		items = append(items, item)
	}
	return items, rows.Err()
}

func (r *Repository) UpdateChallenge(ctx context.Context, id int64, status, keyAuth, errorMessage string, validated time.Time) error {
	_, err := r.db.ExecContext(ctx, `UPDATE acme_challenges SET status=?, key_auth=?, error=?, validated=?, created_at=? WHERE id=?`, status, keyAuth, errorMessage, validated, time.Now(), id)
	return err
}

func parseACMETime(value string) time.Time {
	formats := []string{time.RFC3339Nano, time.RFC3339, "2006-01-02 15:04:05.999999999-07:00", "2006-01-02 15:04:05.999999-07:00", "2006-01-02 15:04:05-07:00", "2006-01-02 15:04:05.999999999", "2006-01-02 15:04:05.999999", "2006-01-02 15:04:05"}
	for _, format := range formats {
		if parsed, err := time.Parse(format, value); err == nil {
			return parsed
		}
	}
	return time.Time{}
}

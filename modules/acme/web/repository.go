package web

import (
	"context"
	"crypto/x509"
	"database/sql"
	"encoding/pem"
	acme "github.com/lsongdev/ohmyserver/modules/acme"
	"strings"
	"time"
)

type Repository struct{ db *sql.DB }

type StoredCertificate = acme.StoredCertificate
type CertificateSummary = acme.CertificateSummary
type TLSMaterial = acme.TLSMaterial

func NewRepository(db *sql.DB) *Repository { return &Repository{db: db} }

// InitializeModule creates every table owned by ACME. The application mounts
// the feature through this single entry point instead of knowing its internal
// schema split across certificates, accounts, orders, challenges, and issuances.
func (r *Repository) InitializeModule(ctx context.Context) error {
	initializers := []func(context.Context) error{
		r.Initialize,
		r.InitializeProviders,
		r.InitializeAccounts,
		r.InitializeOrders,
		r.InitializeAuthorizations,
		r.InitializeChallenges,
		r.InitializeIssuances,
	}
	for _, initialize := range initializers {
		if err := initialize(ctx); err != nil {
			return err
		}
	}
	return r.SeedProviders(ctx)
}

func (r *Repository) Initialize(ctx context.Context) error {
	_, err := r.db.ExecContext(ctx, `
		CREATE TABLE IF NOT EXISTS certificates (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			order_id INTEGER DEFAULT 0,
			domains TEXT NOT NULL,
			csr_pem TEXT NOT NULL,
			private_key_pem TEXT NOT NULL,
			cert_pem TEXT,
			status TEXT NOT NULL DEFAULT 'pending',
			created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
			updated_at DATETIME DEFAULT CURRENT_TIMESTAMP
		);
		CREATE INDEX IF NOT EXISTS idx_certificates_status ON certificates(status);
		CREATE INDEX IF NOT EXISTS idx_certificates_order_id ON certificates(order_id);
	`)
	return err
}

func (r *Repository) Create(ctx context.Context, domains, csrPEM, privateKeyPEM string) (*StoredCertificate, error) {
	now := time.Now()
	result, err := r.db.ExecContext(ctx, `INSERT INTO certificates (domains, csr_pem, private_key_pem, status, created_at, updated_at) VALUES (?, ?, ?, 'pending', ?, ?)`, domains, csrPEM, privateKeyPEM, now, now)
	if err != nil {
		return nil, err
	}
	id, err := result.LastInsertId()
	if err != nil {
		return nil, err
	}
	return &StoredCertificate{ID: id, Domains: domains, CSRPem: csrPEM, PrivateKeyPEM: privateKeyPEM, Status: "pending", CreatedAt: now.Format("2006-01-02 15:04:05"), UpdatedAt: now.Format("2006-01-02 15:04:05")}, nil
}

func (r *Repository) Get(ctx context.Context, id int64) (*StoredCertificate, error) {
	var item StoredCertificate
	var certPEM sql.NullString
	err := r.db.QueryRowContext(ctx, `SELECT id, order_id, domains, csr_pem, private_key_pem, cert_pem, status, created_at, updated_at FROM certificates WHERE id = ?`, id).Scan(&item.ID, &item.OrderID, &item.Domains, &item.CSRPem, &item.PrivateKeyPEM, &certPEM, &item.Status, &item.CreatedAt, &item.UpdatedAt)
	if err != nil {
		return nil, err
	}
	if certPEM.Valid {
		item.CertPEM = certPEM.String
	}
	return &item, nil
}

func (r *Repository) List(ctx context.Context) ([]StoredCertificate, error) {
	rows, err := r.db.QueryContext(ctx, `SELECT id, order_id, domains, csr_pem, private_key_pem, cert_pem, status, created_at, updated_at FROM certificates ORDER BY created_at DESC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := make([]StoredCertificate, 0)
	for rows.Next() {
		var item StoredCertificate
		var certPEM sql.NullString
		if err := rows.Scan(&item.ID, &item.OrderID, &item.Domains, &item.CSRPem, &item.PrivateKeyPEM, &certPEM, &item.Status, &item.CreatedAt, &item.UpdatedAt); err != nil {
			return nil, err
		}
		if certPEM.Valid {
			item.CertPEM = certPEM.String
		}
		items = append(items, item)
	}
	return items, rows.Err()
}

func (r *Repository) Update(ctx context.Context, id, orderID int64, certPEM, status string) error {
	_, err := r.db.ExecContext(ctx, `UPDATE certificates SET order_id = ?, cert_pem = ?, status = ?, updated_at = ? WHERE id = ?`, orderID, certPEM, status, time.Now(), id)
	return err
}

func (r *Repository) UpdateStatus(ctx context.Context, id int64, status string) error {
	_, err := r.db.ExecContext(ctx, `UPDATE certificates SET status = ?, updated_at = ? WHERE id = ?`, status, time.Now(), id)
	return err
}

func (r *Repository) Delete(ctx context.Context, id int64) error {
	_, err := r.db.ExecContext(ctx, `DELETE FROM certificates WHERE id = ?`, id)
	return err
}

func (r *Repository) GetSummary(ctx context.Context, id int64) (CertificateSummary, error) {
	var item CertificateSummary
	var certPEM sql.NullString
	err := r.db.QueryRowContext(ctx, `SELECT id, domains, cert_pem, status, created_at, updated_at FROM certificates WHERE id = ?`, id).Scan(&item.ID, &item.Domains, &certPEM, &item.Status, &item.CreatedAt, &item.UpdatedAt)
	if certPEM.Valid {
		item.CertPEM = certPEM.String
	}
	return item, err
}

func (r *Repository) ListSummaries(ctx context.Context) ([]CertificateSummary, error) {
	rows, err := r.db.QueryContext(ctx, `SELECT id, domains, cert_pem, status, created_at, updated_at FROM certificates ORDER BY created_at DESC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := make([]CertificateSummary, 0)
	for rows.Next() {
		var item CertificateSummary
		var certPEM sql.NullString
		if err := rows.Scan(&item.ID, &item.Domains, &certPEM, &item.Status, &item.CreatedAt, &item.UpdatedAt); err != nil {
			return nil, err
		}
		if certPEM.Valid {
			item.CertPEM = certPEM.String
		}
		items = append(items, item)
	}
	return items, rows.Err()
}

func (r *Repository) LoadTLSMaterial(ctx context.Context, id int64) (TLSMaterial, error) {
	var item TLSMaterial
	var certPEM sql.NullString
	err := r.db.QueryRowContext(ctx, `SELECT cert_pem, private_key_pem, status FROM certificates WHERE id = ?`, id).Scan(&certPEM, &item.PrivateKeyPEM, &item.Status)
	if certPEM.Valid {
		item.CertPEM = certPEM.String
	}
	return item, err
}

func (r *Repository) ListACMECertificates(ctx context.Context) ([]Certificate, error) {
	items, err := r.ListSummaries(ctx)
	if err != nil {
		return nil, err
	}
	result := make([]Certificate, 0, len(items))
	for _, item := range items {
		cert := Certificate{ID: item.ID, Domains: splitDomains(item.Domains), Status: item.Status, CreatedAt: item.CreatedAt, UpdatedAt: item.UpdatedAt, DNSNames: []string{}, DaysUntilExpiry: -1}
		block, _ := pem.Decode([]byte(item.CertPEM))
		if block != nil {
			if parsed, parseErr := x509.ParseCertificate(block.Bytes); parseErr == nil {
				cert.CommonName = parsed.Subject.CommonName
				cert.DNSNames = parsed.DNSNames
				cert.ExpiresAt = parsed.NotAfter
				cert.DaysUntilExpiry = int(time.Until(parsed.NotAfter).Hours() / 24)
			}
		}
		result = append(result, cert)
	}
	return result, nil
}

func splitDomains(value string) []string {
	if strings.TrimSpace(value) == "" {
		return []string{}
	}
	parts := strings.Split(value, ",")
	result := make([]string, 0, len(parts))
	for _, part := range parts {
		if value := strings.TrimSpace(part); value != "" {
			result = append(result, value)
		}
	}
	return result
}

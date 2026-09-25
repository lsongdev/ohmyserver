package web

import (
	"context"
	"database/sql"
	"errors"
	"path/filepath"
	"testing"

	_ "modernc.org/sqlite"
)

func TestInitializeModuleOwnsCompleteAndRepeatableSchema(t *testing.T) {
	db, err := sql.Open("sqlite", filepath.Join(t.TempDir(), "acme.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })

	repository := NewRepository(db)
	ctx := context.Background()
	if err := repository.InitializeModule(ctx); err != nil {
		t.Fatalf("initialize module: %v", err)
	}
	if _, err := db.ExecContext(ctx, `INSERT INTO acme_issuances (certificate_id, account_id, domains, challenge_type, status, step) VALUES (1, 1, 'example.test', 'http-01', 'running', 'challenge')`); err != nil {
		t.Fatal(err)
	}
	if err := repository.InitializeModule(ctx); err != nil {
		t.Fatalf("initialize module a second time: %v", err)
	}
	var status string
	if err := db.QueryRowContext(ctx, `SELECT status FROM acme_issuances WHERE domains='example.test'`).Scan(&status); err != nil {
		t.Fatal(err)
	}
	if status != "running" {
		t.Fatalf("schema initialization unexpectedly performed recovery: status=%q", status)
	}
	if err := repository.RecoverInterruptedIssuances(ctx); err != nil {
		t.Fatalf("recover interrupted issuances: %v", err)
	}
	if err := db.QueryRowContext(ctx, `SELECT status FROM acme_issuances WHERE domains='example.test'`).Scan(&status); err != nil {
		t.Fatal(err)
	}
	if status != "failed" {
		t.Fatalf("recovered issuance status=%q, want failed", status)
	}

	for _, table := range []string{
		"certificates", "acme_providers", "acme_accounts", "acme_orders",
		"acme_authorizations", "acme_challenges", "acme_issuances",
	} {
		t.Run(table, func(t *testing.T) {
			var exists bool
			if err := db.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM sqlite_master WHERE type='table' AND name=?)`, table).Scan(&exists); err != nil {
				t.Fatal(err)
			}
			if !exists {
				t.Fatalf("ACME-owned table %q was not initialized", table)
			}
		})
	}
}

func TestCreateRenewalIssuancePreventsConcurrentRenewals(t *testing.T) {
	db, err := sql.Open("sqlite", filepath.Join(t.TempDir(), "renewal.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	repository := NewRepository(db)
	ctx := context.Background()
	if err := repository.InitializeModule(ctx); err != nil {
		t.Fatal(err)
	}
	if _, err := db.ExecContext(ctx, `INSERT INTO certificates (domains, csr_pem, private_key_pem, cert_pem, status) VALUES ('example.test', 'csr', 'key', 'certificate', 'active')`); err != nil {
		t.Fatal(err)
	}
	first := &Issuance{CertificateID: 1, AccountID: 1, Domains: "example.test", ChallengeType: "http-01"}
	if err := repository.CreateRenewalIssuance(ctx, first); err != nil {
		t.Fatalf("create renewal issuance: %v", err)
	}
	var certStatus, certPEM string
	if err := db.QueryRowContext(ctx, `SELECT status, cert_pem FROM certificates WHERE id=1`).Scan(&certStatus, &certPEM); err != nil {
		t.Fatal(err)
	}
	if certStatus != "active" || certPEM != "certificate" {
		t.Fatalf("queued renewal changed current certificate: status=%q cert=%q", certStatus, certPEM)
	}
	second := &Issuance{CertificateID: 1, AccountID: 1, Domains: "example.test", ChallengeType: "http-01"}
	if err := repository.CreateRenewalIssuance(ctx, second); !errors.Is(err, ErrIssuanceInProgress) {
		t.Fatalf("concurrent renewal error=%v, want ErrIssuanceInProgress", err)
	}
	if err := repository.UpdateIssuance(ctx, first.ID, "failed", "test", 0, errors.New("failed")); err != nil {
		t.Fatal(err)
	}
	if err := repository.CreateRenewalIssuance(ctx, second); err != nil {
		t.Fatalf("retry renewal after failed attempt: %v", err)
	}
}

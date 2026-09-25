package web

import (
	"context"
	"database/sql"
	"path/filepath"
	"testing"

	_ "modernc.org/sqlite"
)

func TestInitializeModuleCreatesAllDNSTables(t *testing.T) {
	db, err := sql.Open("sqlite", filepath.Join(t.TempDir(), "dns.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })

	repository := NewRepository(db)
	ctx := context.Background()
	for i := 0; i < 2; i++ {
		if err := repository.InitializeModule(ctx); err != nil {
			t.Fatalf("initialize DNS module iteration %d: %v", i+1, err)
		}
	}
	for _, table := range []string{
		"dns_provider_connections", "dns_settings", "dns_migrations", "dns_zones",
		"dns_server_records", "dns_subscriptions", "dns_rules",
	} {
		var exists bool
		if err := db.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM sqlite_master WHERE type='table' AND name=?)`, table).Scan(&exists); err != nil {
			t.Fatal(err)
		}
		if !exists {
			t.Errorf("DNS-owned table %q was not initialized", table)
		}
	}
}

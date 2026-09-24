package web

import (
	"context"
	"database/sql"
	"errors"
	"testing"

	_ "modernc.org/sqlite"
)

func TestLocalServiceRollsBackWhenHostPolicyRejectsSettings(t *testing.T) {
	db, err := sql.Open("sqlite", "file:dns-policy-rollback-test?mode=memory&cache=shared")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	repo := NewRepository(db)
	ctx := context.Background()
	if err := repo.InitializeModule(ctx); err != nil {
		t.Fatal(err)
	}
	service := NewLocalService(ctx, repo, t.TempDir())
	t.Cleanup(func() { _ = service.Close() })
	var policyCalls []bool
	service.SetApplyPolicy(func(_ context.Context, settings Settings) error {
		policyCalls = append(policyCalls, settings.Enabled)
		if settings.Enabled {
			return errors.New("listener conflict")
		}
		return nil
	})
	settings, err := repo.GetSettings(ctx)
	if err != nil {
		t.Fatal(err)
	}
	settings.Enabled = true
	settings.UDPAddr = "127.0.0.1:0"
	if err := service.UpdateDNSConfiguration(ctx, *settings); err == nil {
		t.Fatal("expected host policy rejection")
	}
	stored, err := repo.GetSettings(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if stored.Enabled {
		t.Fatalf("settings should have rolled back to disabled, got %+v", stored)
	}
	if len(policyCalls) != 2 || !policyCalls[0] || policyCalls[1] {
		t.Fatalf("policy should apply new settings then restore old settings: %v", policyCalls)
	}
}

package web

import (
	"context"
	"database/sql"
	"errors"
	"testing"

	_ "modernc.org/sqlite"
)

func TestLocalServiceRollsBackWhenHostPolicyRejectsSettings(t *testing.T) {
	db, err := sql.Open("sqlite", "file:ntp-policy-rollback-test?mode=memory&cache=shared")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	repo := NewRepository(db)
	ctx := context.Background()
	if err := repo.Initialize(ctx); err != nil {
		t.Fatal(err)
	}
	var policyCalls []bool
	service := NewLocalService(repo, func(_ context.Context, settings Settings) error {
		policyCalls = append(policyCalls, settings.Enabled)
		if settings.Enabled {
			return errors.New("listener conflict")
		}
		return nil
	})
	t.Cleanup(func() { _ = service.Close() })
	settings, err := repo.Get(ctx)
	if err != nil {
		t.Fatal(err)
	}
	settings.Enabled = true
	settings.ListenAddr = "127.0.0.1:0"
	if err := service.UpdateNTPConfiguration(ctx, *settings); err == nil {
		t.Fatal("expected host policy rejection")
	}
	stored, err := repo.Get(ctx)
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

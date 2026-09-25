package web

import (
	"context"
	"database/sql"
	"errors"
	"testing"

	dhcp "github.com/lsongdev/ohmyserver/modules/dhcp"
	_ "modernc.org/sqlite"
)

func TestLocalServiceRollsBackWhenHostPolicyRejectsSettings(t *testing.T) {
	db, err := sql.Open("sqlite", "file:dhcp-policy-rollback-test?mode=memory&cache=shared")
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
	service := NewLocalService(ctx, repo, func(_ context.Context, settings Settings) error {
		policyCalls = append(policyCalls, settings.Enabled)
		if settings.Enabled {
			return errors.New("listener conflict")
		}
		return nil
	})
	settings := dhcp.Settings{
		Enabled: true, ListenAddr: ":1067", ServerIP: "192.168.1.1", SubnetMask: "255.255.255.0",
		PoolStart: "192.168.1.100", PoolEnd: "192.168.1.200", Router: "192.168.1.1",
		DNSServers: "1.1.1.1", LeaseSeconds: 3600, RenewalSeconds: 1800, RebindingSeconds: 3150,
	}
	if err := service.UpdateDHCPConfiguration(ctx, settings); err == nil {
		t.Fatal("expected host policy rejection")
	}
	stored, err := repo.Settings(ctx)
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

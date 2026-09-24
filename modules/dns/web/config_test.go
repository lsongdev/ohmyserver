package web

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	dnsmodule "github.com/lsongdev/ohmyserver/modules/dns"
)

func TestBuildConfigAssemblesOwnedDNSData(t *testing.T) {
	_, service := newTestHandler(t)
	ctx := context.Background()
	settings, err := service.repo.GetSettings(ctx)
	if err != nil {
		t.Fatal(err)
	}
	settings.Enabled, settings.UDPAddr, settings.Upstreams = true, "127.0.0.1:0", ""
	if err := service.repo.UpdateSettings(ctx, *settings); err != nil {
		t.Fatal(err)
	}
	zone, err := service.repo.CreateZone(ctx, "example.test", "", true)
	if err != nil {
		t.Fatal(err)
	}
	record := &dnsmodule.LocalRecord{ZoneID: zone.ID, Name: "host", Type: "A", TTL: 60, Value: "192.0.2.10", Enabled: true}
	if err := service.repo.SaveRecord(ctx, record); err != nil {
		t.Fatal(err)
	}
	if _, err := service.repo.CreateRule(ctx, "||ads.example^"); err != nil {
		t.Fatal(err)
	}

	cfg, enabled, err := BuildConfig(ctx, service.repo, t.TempDir(), false)
	if err != nil {
		t.Fatal(err)
	}
	if !enabled || len(cfg.Listens) != 2 || len(cfg.Domains) != 1 || cfg.Domains[0].Domain != zone.Name || len(cfg.Domains[0].Records) != 1 || len(cfg.Filters.Rules) != 1 {
		t.Fatalf("unexpected built DNS config: enabled=%v config=%#v", enabled, cfg)
	}
}

func TestBuildConfigRefreshesSubscriptionIntoModuleDirectory(t *testing.T) {
	_, service := newTestHandler(t)
	ctx := context.Background()
	settings, err := service.repo.GetSettings(ctx)
	if err != nil {
		t.Fatal(err)
	}
	settings.Enabled, settings.UDPAddr, settings.Upstreams = true, "127.0.0.1:0", ""
	if err := service.repo.UpdateSettings(ctx, *settings); err != nil {
		t.Fatal(err)
	}
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { _, _ = w.Write([]byte("||ads.example^\n")) }))
	defer upstream.Close()
	item := &dnsmodule.Subscription{Name: "Block list", Kind: "block", URL: upstream.URL, Enabled: true, RefreshHours: 24}
	if err := service.repo.SaveSubscription(ctx, item); err != nil {
		t.Fatal(err)
	}
	listDir := filepath.Join(t.TempDir(), "lists")
	cfg, _, err := BuildConfig(ctx, service.repo, listDir, true)
	if err != nil {
		t.Fatal(err)
	}
	stored, err := service.repo.Subscription(ctx, item.ID)
	if err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(stored.LocalPath)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(data), "||ads.example^") || len(cfg.Filters.Blocklists) != 1 {
		t.Fatalf("subscription not assembled: body=%q config=%#v", data, cfg.Filters)
	}
}

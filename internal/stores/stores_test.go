package stores

import (
	"context"
	"errors"
	"path/filepath"
	"testing"
	"time"

	"github.com/lsongdev/docker-go/apps"
	"github.com/lsongdev/yabber"
	_ "modernc.org/sqlite"
)

func TestDockerAppsAndGatewayShareTheDatabase(t *testing.T) {
	ctx := context.Background()
	db, err := Open(filepath.Join(t.TempDir(), "ohmyserver.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	dockerApps, err := NewDockerApps(ctx, db)
	if err != nil {
		t.Fatal(err)
	}
	gateway, err := NewGatewayConfig(ctx, db)
	if err != nil {
		t.Fatal(err)
	}
	// Migrating again is a no-op.
	if _, err := NewDockerApps(ctx, db); err != nil {
		t.Fatal(err)
	}

	if _, err := dockerApps.GetApp(ctx, "notes"); !errors.Is(err, apps.ErrNotInstalled) {
		t.Fatalf("missing app: %v", err)
	}
	now := time.Now().UTC().Truncate(time.Microsecond)
	r := apps.Record{ID: "notes", Version: "1.0", Port: 9000, Domain: "notes.example.com", Values: map[string]string{"K": "v"}, InstalledAt: now, UpdatedAt: now}
	if err := dockerApps.PutApp(ctx, r); err != nil {
		t.Fatal(err)
	}
	r.Version = "2.0"
	dockerApps.PutApp(ctx, r)
	got, err := dockerApps.GetApp(ctx, "notes")
	if err != nil || got.Version != "2.0" || got.Values["K"] != "v" || !got.InstalledAt.Equal(now) {
		t.Fatalf("record = %+v, %v", got, err)
	}
	if list, _ := dockerApps.ListApps(ctx); len(list) != 1 {
		t.Fatalf("list = %+v", list)
	}
	if err := dockerApps.DeleteApp(ctx, "notes"); err != nil {
		t.Fatal(err)
	}

	if _, ok, err := gateway.Load(ctx); ok || err != nil {
		t.Fatalf("empty gateway config: %v %v", ok, err)
	}
	config := yabber.Config{
		Listeners: []yabber.Listener{{Name: "http", Address: ":80"}},
		Sites:     []yabber.Site{{Name: "app-notes", Hosts: []string{"notes.example.com"}, Handler: yabber.Handler{Proxy: &yabber.Proxy{Upstream: "http://127.0.0.1:9000"}}}},
	}
	if err := gateway.Save(config); err != nil {
		t.Fatal(err)
	}
	loaded, ok, err := gateway.Load(ctx)
	if err != nil || !ok || len(loaded.Sites) != 1 || loaded.Sites[0].Proxy == nil || loaded.Sites[0].Proxy.Upstream != "http://127.0.0.1:9000" {
		t.Fatalf("loaded = %+v, %v, %v", loaded, ok, err)
	}
}

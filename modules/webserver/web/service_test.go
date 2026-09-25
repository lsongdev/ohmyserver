package web

import (
	"context"
	"database/sql"
	"errors"
	"testing"

	webservermodule "github.com/lsongdev/ohmyserver/modules/webserver"
	"github.com/lsongdev/ohmyserver/modules/webserver/web/repository"
	_ "modernc.org/sqlite"
)

func newServiceRepository(t *testing.T) *repository.Repository {
	t.Helper()
	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	db.SetMaxOpenConns(1)
	t.Cleanup(func() { _ = db.Close() })
	repo := repository.NewRepository(db)
	if err := repo.Initialize(context.Background()); err != nil {
		t.Fatal(err)
	}
	return repo
}

func TestLocalServiceOwnsWebserverCRUDAndAppliesConfiguration(t *testing.T) {
	ctx := context.Background()
	repo := newServiceRepository(t)
	applied := 0
	service := NewLocalService(repo, nil, nil, func(context.Context) error { applied++; return nil })
	listener, err := service.CreateWebListenerAPI(ctx, webservermodule.Listener{Name: "http", Address: ":8080", Protocol: "http", Enabled: true})
	if err != nil {
		t.Fatal(err)
	}
	if listener.ID == 0 || listener.MinTLSVersion != "TLS1.2" {
		t.Fatalf("listener = %#v", listener)
	}
	host, err := service.CreateWebHostAPI(ctx, webservermodule.Host{Name: "default", Hostnames: "example.com", Enabled: true, ListenerIDs: []int64{listener.ID}})
	if err != nil {
		t.Fatal(err)
	}
	route, err := service.CreateWebRouteAPI(ctx, host.ID, webservermodule.Route{Name: "home", Path: "/", HandlerType: "static_response", ResponseStatus: 200, ResponseBody: "ok", Enabled: true})
	if err != nil {
		t.Fatal(err)
	}
	policy, err := service.CreateWebPolicyAPI(ctx, webservermodule.Policy{Name: "public", RateRequestsPerSecond: 5, RateBurst: 10, RateKey: "client_ip"})
	if err != nil {
		t.Fatal(err)
	}
	if err := service.UpdateWebRouteAPI(ctx, route.ID, webservermodule.Route{Name: "home", Path: "/", HandlerType: "static_response", ResponseStatus: 204, Enabled: true, PolicyIDs: []int64{policy.ID}}); err != nil {
		t.Fatal(err)
	}
	hosts, err := service.WebHosts(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(hosts) != 1 || len(hosts[0].Routes) != 1 || hosts[0].Routes[0].ResponseStatus != 204 || len(hosts[0].Routes[0].PolicyNames) != 1 {
		t.Fatalf("hosts = %#v", hosts)
	}
	if applied != 5 {
		t.Fatalf("configuration applied %d times, want 5", applied)
	}
}

func TestLocalServiceRollsBackCreateWhenHostApplyPolicyRejects(t *testing.T) {
	repo := newServiceRepository(t)
	service := NewLocalService(repo, nil, nil, func(context.Context) error { return errors.New("port conflicts with SSH") })
	if _, err := service.CreateWebListenerAPI(context.Background(), webservermodule.Listener{Name: "http", Address: ":8080", Protocol: "http"}); err == nil {
		t.Fatal("expected apply policy failure")
	}
	items, err := repo.ListListeners(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 0 {
		t.Fatalf("listener persisted after failed apply: %#v", items)
	}
}

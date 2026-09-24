package web

import (
	"context"
	"database/sql"
	"strings"
	"testing"

	webservermodule "github.com/lsongdev/ohmyserver/modules/webserver"
	"github.com/lsongdev/ohmyserver/modules/webserver/web/repository"
	"github.com/lsongdev/yabber"
	_ "modernc.org/sqlite"
)

func TestRuntimeBuildsYabberConfigFromModuleRepository(t *testing.T) {
	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	db.SetMaxOpenConns(1)
	ctx := context.Background()
	repo := repository.NewRepository(db)
	if err := repo.Initialize(ctx); err != nil {
		t.Fatal(err)
	}
	policyID, err := repo.CreatePolicy(ctx, webservermodule.Policy{Name: "public", RateRequestsPerSecond: 10, RateBurst: 20, RateKey: "client_ip", RateMaxKeys: 1000, RateIdleTimeoutSeconds: 300, ConcurrencyMax: 50})
	if err != nil {
		t.Fatal(err)
	}
	listenerID, err := repo.CreateListener(ctx, webservermodule.Listener{Name: "http", Address: ":8080", Protocol: "http", Enabled: true, TrustedProxies: "127.0.0.1/32\n10.0.0.0/8", PolicyIDs: []int64{policyID}})
	if err != nil {
		t.Fatal(err)
	}
	hostID, err := repo.CreateHost(ctx, webservermodule.Host{Name: "default", Hostnames: "example.com\nwww.example.com", Enabled: true, ListenerIDs: []int64{listenerID}, PolicyIDs: []int64{policyID}})
	if err != nil {
		t.Fatal(err)
	}
	for _, route := range []webservermodule.Route{
		{Name: "home", HostID: hostID, Enabled: true, Path: "/", HandlerType: "static_response", ResponseStatus: 200, ResponseBody: "hello", PolicyIDs: []int64{policyID}},
		{Name: "worker", HostID: hostID, Enabled: true, Path: "/worker", HandlerType: "worker", WorkerScript: `export default { fetch() { return new Response("hello"); } }`},
	} {
		if _, err := repo.CreateRoute(ctx, route); err != nil {
			t.Fatal(err)
		}
	}

	runtime := NewRuntime(ctx, repo, nil, nil)
	config, err := runtime.BuildConfig(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(config.Listeners) != 1 || len(config.Hosts) != 1 || len(config.Policies) != 1 {
		t.Fatalf("config counts: listeners=%d hosts=%d policies=%d", len(config.Listeners), len(config.Hosts), len(config.Policies))
	}
	if strings.Join(config.Listeners[0].TrustedProxies, ",") != "127.0.0.1/32,10.0.0.0/8" || strings.Join(config.Listeners[0].Policies, ",") != "public" {
		t.Fatalf("listener config = %#v", config.Listeners[0])
	}
	host := config.Hosts[0]
	if strings.Join(host.Hostnames, ",") != "example.com,www.example.com" || len(host.Routes) != 2 {
		t.Fatalf("host config = %#v", host)
	}
	if host.Routes[0].Handle.StaticResponse == nil || host.Routes[1].Handle.Worker == nil || host.Routes[0].Policies[0] != "public" {
		t.Fatalf("route config = %#v", host.Routes)
	}
	instance, err := yabber.New(config)
	if err != nil {
		t.Fatalf("compile runtime config: %v", err)
	}
	if err := instance.Close(); err != nil {
		t.Fatal(err)
	}
}

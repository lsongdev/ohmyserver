package host

import (
	"context"
	"database/sql"
	"strings"
	"testing"

	"github.com/lsongdev/ohmyserver/config"
	webmodule "github.com/lsongdev/ohmyserver/modules/webserver"
	"github.com/lsongdev/yabber"
	_ "modernc.org/sqlite"
)

func TestBuildYabberConfigFromDatabase(t *testing.T) {
	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	db.SetMaxOpenConns(1)

	cfg := &config.Config{}
	server := &Server{db: db, config: cfg}
	if err := server.ACMERepository().Initialize(t.Context()); err != nil {
		t.Fatal(err)
	}
	if err := server.initWebServerSchema(); err != nil {
		t.Fatal(err)
	}

	repository := server.WebServerRepository()
	listeners, err := repository.ListListeners(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	if len(listeners) != 1 || listeners[0].Address != ":8081" {
		t.Fatalf("listeners = %#v", listeners)
	}
	policyID, err := repository.CreatePolicy(t.Context(), webmodule.Policy{
		Name:                   "public",
		MaxBodyBytes:           1 << 20,
		RateRequestsPerSecond:  10,
		RateBurst:              20,
		RateKey:                "client_ip",
		RateMaxKeys:            1000,
		RateIdleTimeoutSeconds: 300,
		ConcurrencyMax:         50,
	})
	if err != nil {
		t.Fatal(err)
	}
	listeners[0].TrustedProxies = "127.0.0.1/32\n10.0.0.0/8"
	listeners[0].PolicyIDs = []int64{policyID}
	if err := repository.UpdateListener(t.Context(), listeners[0]); err != nil {
		t.Fatal(err)
	}

	hostID, err := repository.CreateHost(t.Context(), webmodule.Host{
		Name:        "example",
		Hostnames:   "example.com\nwww.example.com",
		Enabled:     true,
		ListenerIDs: []int64{listeners[0].ID},
		PolicyIDs:   []int64{policyID},
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := repository.CreateRoute(t.Context(), webmodule.Route{
		HostID:         hostID,
		Name:           "home",
		Enabled:        true,
		Path:           "/",
		HandlerType:    "static_response",
		ResponseStatus: 200,
		ResponseBody:   "hello",
		PolicyIDs:      []int64{policyID},
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := repository.CreateRoute(t.Context(), webmodule.Route{
		HostID: hostID, Name: "worker", Enabled: true, Path: "/worker",
		HandlerType: "worker", WorkerScript: `export default {
			fetch(request, env) { return new Response("hello"); }
		}`,
	}); err != nil {
		t.Fatal(err)
	}
	if err := repository.DeleteListener(t.Context(), listeners[0].ID); err == nil {
		t.Fatal("listener referenced by a virtual host was deleted")
	}

	runtimeConfig, err := server.webserverRuntime().BuildConfig(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(runtimeConfig.Listeners) != 1 || len(runtimeConfig.Hosts) != 1 {
		t.Fatalf("runtime config = %#v", runtimeConfig)
	}
	if len(runtimeConfig.Policies) != 1 || runtimeConfig.Policies[0].RateLimit == nil ||
		runtimeConfig.Policies[0].Concurrency == nil {
		t.Fatalf("policies = %#v", runtimeConfig.Policies)
	}
	if got := runtimeConfig.Listeners[0].TrustedProxies; strings.Join(got, ",") != "127.0.0.1/32,10.0.0.0/8" {
		t.Fatalf("trusted proxies = %#v", got)
	}
	if strings.Join(runtimeConfig.Listeners[0].Policies, ",") != "public" {
		t.Fatalf("listener policies = %#v", runtimeConfig.Listeners[0].Policies)
	}
	host := runtimeConfig.Hosts[0]
	if strings.Join(host.Hostnames, ",") != "example.com,www.example.com" {
		t.Fatalf("hostnames = %#v", host.Hostnames)
	}
	if len(host.Routes) != 2 || host.Routes[0].Handle.StaticResponse == nil ||
		host.Routes[1].Handle.Worker == nil {
		t.Fatalf("routes = %#v", host.Routes)
	}
	if strings.Join(host.Policies, ",") != "public" ||
		strings.Join(host.Routes[0].Policies, ",") != "public" {
		t.Fatalf("policy bindings = host %#v route %#v", host.Policies, host.Routes[0].Policies)
	}
	if err := repository.DeletePolicy(t.Context(), policyID); err == nil {
		t.Fatal("bound policy was deleted")
	}
	instance, err := yabber.New(runtimeConfig)
	if err != nil {
		t.Fatalf("compile runtime config: %v", err)
	}
	if err := instance.Close(); err != nil {
		t.Fatal(err)
	}
}

func TestSeedDefaultWebWorkerIsIdempotent(t *testing.T) {
	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	db.SetMaxOpenConns(1)

	s := &Server{db: db, config: &config.Config{}}
	if err := s.ACMERepository().Initialize(t.Context()); err != nil {
		t.Fatal(err)
	}
	if err := s.initServicesSchema(); err != nil {
		t.Fatal(err)
	}
	if err := s.initWebServerSchema(); err != nil {
		t.Fatal(err)
	}
	if err := s.SeedDefaultWebWorker(); err != nil {
		t.Fatal(err)
	}
	if err := s.SeedDefaultWebWorker(); err != nil {
		t.Fatal(err)
	}

	runtimeConfig, err := s.webserverRuntime().BuildConfig(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(runtimeConfig.Listeners) != 1 || runtimeConfig.Listeners[0].Address != ":80" {
		t.Fatalf("listeners = %#v", runtimeConfig.Listeners)
	}
	if len(runtimeConfig.Hosts) != 1 || len(runtimeConfig.Hosts[0].Routes) != 1 {
		t.Fatalf("hosts = %#v", runtimeConfig.Hosts)
	}
	worker := runtimeConfig.Hosts[0].Routes[0].Handle.Worker
	if worker == nil || !strings.Contains(worker.Script, "OhMyServer Web Server") {
		t.Fatalf("worker = %#v", worker)
	}
}

func TestEnsureWebServerColumnWithSingleConnection(t *testing.T) {
	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	db.SetMaxOpenConns(1)
	if _, err := db.Exec(`CREATE TABLE web_listeners (id INTEGER PRIMARY KEY AUTOINCREMENT,name TEXT NOT NULL UNIQUE,address TEXT NOT NULL,protocol TEXT NOT NULL,enabled INTEGER NOT NULL DEFAULT 1,min_tls_version TEXT NOT NULL DEFAULT 'TLS1.2',default_certificate_id INTEGER,created_at DATETIME DEFAULT CURRENT_TIMESTAMP,updated_at DATETIME DEFAULT CURRENT_TIMESTAMP)`); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO web_listeners(id,name,address,protocol) VALUES(1,'legacy',':80','http')`); err != nil {
		t.Fatal(err)
	}
	s := &Server{db: db}
	if err := s.initWebServerSchema(); err != nil {
		t.Fatal(err)
	}
	var value string
	if err := db.QueryRow(`SELECT trusted_proxies FROM web_listeners WHERE id = 1`).Scan(&value); err != nil {
		t.Fatal(err)
	}
	if value != "" {
		t.Fatalf("trusted_proxies = %q; want empty default", value)
	}
}

func TestLegacyWebCertificateReferencesAreDetachedByApplicationMigration(t *testing.T) {
	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	db.SetMaxOpenConns(1)

	statements := []string{
		`CREATE TABLE certificates (id INTEGER PRIMARY KEY)`,
		`INSERT INTO certificates(id) VALUES(7)`,
		`CREATE TABLE web_listeners (id INTEGER PRIMARY KEY AUTOINCREMENT,name TEXT NOT NULL UNIQUE,address TEXT NOT NULL,protocol TEXT NOT NULL,enabled INTEGER NOT NULL DEFAULT 1,min_tls_version TEXT NOT NULL DEFAULT 'TLS1.2',default_certificate_id INTEGER REFERENCES certificates(id),created_at DATETIME DEFAULT CURRENT_TIMESTAMP,updated_at DATETIME DEFAULT CURRENT_TIMESTAMP)`,
		`INSERT INTO web_listeners(id,name,address,protocol,default_certificate_id) VALUES(1,'https',':443','https',7)`,
		`CREATE TABLE web_hosts (id INTEGER PRIMARY KEY AUTOINCREMENT,name TEXT NOT NULL UNIQUE,hostnames TEXT NOT NULL,enabled INTEGER NOT NULL DEFAULT 1,certificate_id INTEGER REFERENCES certificates(id),force_https INTEGER NOT NULL DEFAULT 0,created_at DATETIME DEFAULT CURRENT_TIMESTAMP,updated_at DATETIME DEFAULT CURRENT_TIMESTAMP)`,
		`INSERT INTO web_hosts(id,name,hostnames,certificate_id) VALUES(2,'example','example.com',7)`,
	}
	for _, statement := range statements {
		if _, err := db.Exec(statement); err != nil {
			t.Fatal(err)
		}
	}

	s := &Server{db: db}
	if err := s.initWebServerSchema(); err != nil {
		t.Fatal(err)
	}
	for _, table := range []string{"web_listeners", "web_hosts"} {
		rows, err := db.Query("PRAGMA foreign_key_list(" + table + ")")
		if err != nil {
			t.Fatal(err)
		}
		for rows.Next() {
			var id, seq int
			var target, from, to, onUpdate, onDelete, match string
			if err := rows.Scan(&id, &seq, &target, &from, &to, &onUpdate, &onDelete, &match); err != nil {
				rows.Close()
				t.Fatal(err)
			}
			if target == "certificates" {
				rows.Close()
				t.Fatalf("%s still references the certificates table", table)
			}
		}
		if err := rows.Err(); err != nil {
			rows.Close()
			t.Fatal(err)
		}
		rows.Close()
	}
	var listenerCertificate, hostCertificate int64
	if err := db.QueryRow(`SELECT default_certificate_id FROM web_listeners WHERE id=1`).Scan(&listenerCertificate); err != nil {
		t.Fatal(err)
	}
	if err := db.QueryRow(`SELECT certificate_id FROM web_hosts WHERE id=2`).Scan(&hostCertificate); err != nil {
		t.Fatal(err)
	}
	if listenerCertificate != 7 || hostCertificate != 7 {
		t.Fatalf("certificate references changed during migration: listener=%d host=%d", listenerCertificate, hostCertificate)
	}
}

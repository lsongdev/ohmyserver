package server

import (
	"database/sql"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/lsongdev/ohmyserver/config"
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
	if err := server.initCertificateSchema(); err != nil {
		t.Fatal(err)
	}
	if err := server.initWebServerSchema(); err != nil {
		t.Fatal(err)
	}

	listeners, err := server.GetAllWebListeners()
	if err != nil {
		t.Fatal(err)
	}
	if len(listeners) != 1 || listeners[0].Address != ":8081" {
		t.Fatalf("listeners = %#v", listeners)
	}
	policyID, err := server.CreateWebPolicy(&WebPolicy{
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
	if err := server.UpdateWebListener(&listeners[0]); err != nil {
		t.Fatal(err)
	}

	hostID, err := server.CreateWebHost(&WebHost{
		Name:        "example",
		Hostnames:   "example.com\nwww.example.com",
		Enabled:     true,
		ListenerIDs: []int64{listeners[0].ID},
		PolicyIDs:   []int64{policyID},
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := server.CreateWebRoute(&WebRoute{
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
	if _, err := server.CreateWebRoute(&WebRoute{
		HostID: hostID, Name: "worker", Enabled: true, Path: "/worker",
		HandlerType: "worker", WorkerScript: `export default {
			fetch(request, env) { return new Response("hello"); }
		}`,
	}); err != nil {
		t.Fatal(err)
	}
	if err := server.DeleteWebListener(listeners[0].ID); err == nil {
		t.Fatal("listener referenced by a virtual host was deleted")
	}

	runtimeConfig, err := server.buildYabberConfig()
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
	if err := server.DeleteWebPolicy(policyID); err == nil {
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
	if err := s.initCertificateSchema(); err != nil {
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

	runtimeConfig, err := s.buildYabberConfig()
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
	if _, err := db.Exec(`CREATE TABLE legacy_web_listener (id INTEGER PRIMARY KEY)`); err != nil {
		t.Fatal(err)
	}
	s := &Server{db: db}
	if err := s.ensureWebServerColumn(
		"legacy_web_listener", "trusted_proxies", "TEXT NOT NULL DEFAULT ''",
	); err != nil {
		t.Fatal(err)
	}
	var value string
	if err := db.QueryRow(`
		SELECT trusted_proxies FROM legacy_web_listener WHERE id = 1
	`).Scan(&value); err != sql.ErrNoRows {
		t.Fatalf("query migrated column: %v", err)
	}
}

func TestWebServerTemplates(t *testing.T) {
	server := &Server{}
	certificateID := int64(1)
	listener := &WebListener{
		ID:                   1,
		Name:                 "https",
		Address:              ":443",
		Protocol:             "https",
		Enabled:              true,
		MinTLSVersion:        "TLS1.2",
		DefaultCertificateID: &certificateID,
	}
	route := WebRoute{
		ID:             1,
		HostID:         1,
		Name:           "app",
		Enabled:        true,
		Path:           "/",
		HandlerType:    "reverse_proxy",
		UpstreamURL:    "http://127.0.0.1:3000",
		ResponseStatus: 0,
	}
	host := &WebHost{
		ID:            1,
		Name:          "example",
		Hostnames:     "example.com",
		Enabled:       true,
		CertificateID: &certificateID,
		ListenerIDs:   []int64{1},
		ListenerNames: []string{"https"},
		Routes:        []WebRoute{route},
	}
	certificates := []Certificate{{ID: 1, Domains: "example.com", Status: "active"}}

	tests := []struct {
		name     string
		template string
		data     H
	}{
		{
			name:     "index",
			template: "webservers/index",
			data: H{
				"listeners": []WebListener{*listener},
				"hosts":     []WebHost{*host},
				"addresses": map[string]string{"https": "0.0.0.0:443"},
				"running":   true,
			},
		},
		{
			name:     "new listener",
			template: "webservers/listener",
			data:     H{"certificates": certificates, "isEdit": false},
		},
		{
			name:     "edit listener",
			template: "webservers/listener",
			data:     H{"listener": listener, "certificates": certificates, "isEdit": true},
		},
		{
			name:     "host overview",
			template: "webservers/host",
			data:     H{"host": host},
		},
		{
			name:     "host edit",
			template: "webservers/host_form",
			data:     H{"host": host, "listeners": []WebListener{*listener}, "certificates": certificates, "isEdit": true},
		},
		{
			name:     "host routes",
			template: "webservers/routes",
			data:     H{"host": host},
		},
		{
			name:     "route",
			template: "webservers/route",
			data:     H{"host": host, "route": &route, "isEdit": true},
		},
		{
			name:     "policy",
			template: "webservers/policy",
			data: H{"policy": &WebPolicy{
				ID: 1, Name: "public", RateKey: "client_ip",
				RateRequestsPerSecond: 10, RateBurst: 20,
			}, "isEdit": true},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			response := httptest.NewRecorder()
			server.Render(response, test.template, test.data)
			if response.Code != 200 {
				t.Fatalf("status = %d, body = %s", response.Code, response.Body.String())
			}
		})
	}
}

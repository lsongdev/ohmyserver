package server

import (
	"bytes"
	"database/sql"
	"io"
	"io/fs"
	"net/http"
	"net/http/httptest"
	"net/url"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
	"time"

	dnsclient "github.com/lsongdev/dns-go/client"
	"github.com/lsongdev/dns-go/packet"
	"github.com/lsongdev/ohmyserver/config"
	webfiles "github.com/lsongdev/ohmyserver/web"
	_ "modernc.org/sqlite"
)

func newDNSTestServer(t *testing.T) *Server {
	t.Helper()
	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	db.SetMaxOpenConns(1)
	t.Cleanup(func() { db.Close() })
	cfg := &config.Config{}
	cfg.Database.Path = filepath.Join(t.TempDir(), "ohmyserver.db")
	cfg.Listen = "127.0.0.1:9000"
	s := &Server{db: db, config: cfg, dnsRuntime: &DNSRuntime{}}
	if err := s.initDNSSchema(); err != nil {
		t.Fatal(err)
	}
	if err := s.initServicesSchema(); err != nil {
		t.Fatal(err)
	}
	if err := s.ensureServicePreferences(); err != nil {
		t.Fatal(err)
	}
	return s
}

func TestDNSConfigAndRecordCRUD(t *testing.T) {
	s := newDNSTestServer(t)
	zone, err := s.CreateDNSZone("Home.Test.", "local services", true)
	if err != nil {
		t.Fatal(err)
	}
	record := &DNSLocalRecord{
		ZoneID:  zone.ID,
		Name:    "router",
		Type:    "A",
		TTL:     120,
		Value:   "192.0.2.1",
		Enabled: true,
	}
	if err := s.saveDNSRecord(record); err != nil {
		t.Fatal(err)
	}
	if record.ID == 0 {
		t.Fatal("record ID was not assigned")
	}

	settings, err := s.GetDNSSettings()
	if err != nil {
		t.Fatal(err)
	}
	if settings.UDPAddr != ":15353" || settings.TCPAddr != ":15353" {
		t.Fatalf("default DNS addresses = udp %q, tcp %q", settings.UDPAddr, settings.TCPAddr)
	}
	settings.Enabled = true
	settings.UDPAddr = "127.0.0.1:0"
	settings.TCPAddr = "127.0.0.1:0"
	settings.Upstreams = ""
	if err := s.UpdateDNSSettings(*settings); err != nil {
		t.Fatal(err)
	}
	cfg, enabled, err := s.buildDNSConfig(t.Context(), false)
	if err != nil {
		t.Fatal(err)
	}
	if !enabled || len(cfg.Domains) != 1 || len(cfg.Domains[0].Records) != 1 {
		t.Fatalf("unexpected DNS config: enabled=%v config=%#v", enabled, cfg)
	}

	if err := s.DeleteDNSZone(zone.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := s.GetDNSRecordByID(record.ID); err != sql.ErrNoRows {
		t.Fatalf("record should be deleted with zone, got %v", err)
	}
}

func TestLegacyDNSProviderConnectionsAreMigrated(t *testing.T) {
	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	db.SetMaxOpenConns(1)
	if _, err := db.Exec(`
		CREATE TABLE dns_providers (
			id INTEGER PRIMARY KEY, name TEXT UNIQUE, type TEXT, config TEXT,
			created_at DATETIME, updated_at DATETIME
		);
		INSERT INTO dns_providers
			(id, name, type, config, created_at, updated_at)
		VALUES (7, 'legacy-cloudflare', 'cloudflare', '{"token":"secret"}',
		        CURRENT_TIMESTAMP, CURRENT_TIMESTAMP);
	`); err != nil {
		t.Fatal(err)
	}
	server := &Server{db: db, config: &config.Config{}, dnsRuntime: &DNSRuntime{}}
	if err := server.initDNSSchema(); err != nil {
		t.Fatal(err)
	}
	connection, err := server.GetDNSProvider(7)
	if err != nil {
		t.Fatal(err)
	}
	if connection.Name != "legacy-cloudflare" || connection.Type != "cloudflare" {
		t.Fatalf("connection = %#v", connection)
	}
}

func TestDNSRuntimeServesLocalRecordsOverUDPAndTCP(t *testing.T) {
	s := newDNSTestServer(t)
	zone, err := s.CreateDNSZone("example.test", "", true)
	if err != nil {
		t.Fatal(err)
	}
	if err := s.saveDNSRecord(&DNSLocalRecord{
		ZoneID:  zone.ID,
		Name:    "host",
		Type:    "A",
		TTL:     60,
		Value:   "192.0.2.25",
		Enabled: true,
	}); err != nil {
		t.Fatal(err)
	}
	settings, err := s.GetDNSSettings()
	if err != nil {
		t.Fatal(err)
	}
	settings.Enabled = true
	settings.UDPAddr = "127.0.0.1:0"
	settings.TCPAddr = "127.0.0.1:0"
	settings.DoHAddr = "127.0.0.1:0"
	settings.Upstreams = ""
	if err := s.UpdateDNSSettings(*settings); err != nil {
		t.Fatal(err)
	}
	if err := s.setServiceDesired("dns", true); err != nil {
		t.Fatal(err)
	}
	if err := s.reloadDNS(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { s.dnsRuntime.Stop() })

	status := s.dnsRuntime.Status()
	if !status.Running || status.UDPAddr == "" || status.TCPAddr == "" || status.DoHAddr == "" {
		t.Fatalf("unexpected runtime status: %#v", status)
	}
	query := packet.NewPacket()
	query.AddQuestionA("host.example.test")

	udp := dnsclient.NewUDPClient(status.UDPAddr)
	udp.Timeout = time.Second
	defer udp.Close()
	response, err := udp.Query(query)
	assertDNSAResponse(t, response, err, "192.0.2.25")

	tcp := dnsclient.NewTCPClient(status.TCPAddr)
	tcp.Timeout = time.Second
	defer tcp.Close()
	response, err = tcp.Query(query)
	assertDNSAResponse(t, response, err, "192.0.2.25")

	httpResponse, err := http.Post(
		"http://"+status.DoHAddr+"/dns-query",
		"application/dns-message",
		bytes.NewReader(query.Bytes()),
	)
	if err != nil {
		t.Fatal(err)
	}
	defer httpResponse.Body.Close()
	body, err := io.ReadAll(httpResponse.Body)
	if err != nil {
		t.Fatal(err)
	}
	response, err = packet.FromBytes(body)
	assertDNSAResponse(t, response, err, "192.0.2.25")
}

func assertDNSAResponse(t *testing.T, response *packet.DNSPacket, err error, expected string) {
	t.Helper()
	if err != nil {
		t.Fatal(err)
	}
	if len(response.Answers) != 1 {
		t.Fatalf("answers = %#v", response.Answers)
	}
	answer, ok := response.Answers[0].(*packet.DNSResourceRecordA)
	if !ok || answer.Address != expected {
		t.Fatalf("answer = %#v", response.Answers[0])
	}
}

func TestDNSTemplates(t *testing.T) {
	s := &Server{}
	now := time.Now()
	zone := &DNSZone{ID: 1, Name: "example.test", Enabled: true}
	record := DNSLocalRecord{ID: 1, ZoneID: 1, Name: "@", Type: "A", TTL: 300, Value: "192.0.2.1", Enabled: true}
	subscription := DNSSubscription{ID: 1, Name: "security", Kind: "block", URL: "https://example.test/list.txt", Enabled: true, RefreshHours: 24, LastUpdated: &now}
	rule := DNSRule{ID: 1, Rule: "||blocked.example^", Enabled: true}
	settings := &DNSSettings{UDPAddr: ":15353", TCPAddr: ":15353", CacheEnabled: true, CacheMinTTL: 60, CacheMaxTTL: 86400, CacheEntries: 10000}
	proxyListener := TCPListener{
		ID: 1, Name: "database", Protocol: "tcp", ListenPort: 3306,
		ForwardHost: "127.0.0.1", ForwardPort: 3307, Status: "active",
		CreatedAt: now, UpdatedAt: now,
	}

	tests := []struct {
		name     string
		template string
		data     H
	}{
		{"overview", "dns/index", H{"settings": settings, "status": DNSRuntimeStatus{}, "zoneCount": 1, "recordCount": 1, "subscriptionCount": 1}},
		{"settings", "dns/settings", H{"settings": settings}},
		{"zones", "dns/zones", H{"zones": []struct {
			DNSZone
			RecordCount int
		}{{DNSZone: *zone, RecordCount: 1}}}},
		{"zone form", "dns/zone_form", H{"zone": zone}},
		{"zone detail", "dns/zone_detail", H{"zone": zone, "records": []DNSLocalRecord{record}}},
		{"record form", "dns/record_form", H{"zone": zone, "record": &record}},
		{"filters", "dns/filters", H{"subscriptions": []DNSSubscription{subscription}, "rules": []DNSRule{rule}}},
		{"subscription form", "dns/subscription_form", H{"subscription": &subscription}},
		{"providers", "dns/providers", H{}},
		{"provider form", "dns/provider_form", H{}},
		{"provider detail", "dns/provider_detail", H{}},
		{"provider zone", "dns/provider_zone", H{}},
		{"proxies", "forward/index", H{"listeners": []H{{
			"ID": 1, "Name": "database", "Protocol": "tcp", "ListenPort": 3306,
			"ForwardHost": "127.0.0.1", "ForwardPort": 3307, "Status": "active",
			"CreatedAt": now.Format("2006-01-02 15:04:05"),
		}}}},
		{"proxy new", "forward/new", H{}},
		{"proxy edit", "forward/edit", H{"listener": &proxyListener}},
		{"proxy detail", "forward/detail", H{"listener": &proxyListener}},
		{"error", "error", H{"error": &Error{Message: "test error"}}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			response := httptest.NewRecorder()
			s.Render(response, test.template, test.data)
			if response.Code != 200 {
				t.Fatalf("status = %d, body = %s", response.Code, response.Body.String())
			}
		})
	}
}

func TestLegacyListenersRedirect(t *testing.T) {
	request := httptest.NewRequest(http.MethodPost, "/listeners/12/edit?source=bookmark", nil)
	response := httptest.NewRecorder()

	(&Server{}).LegacyListenersRedirect(response, request)

	if response.Code != http.StatusPermanentRedirect {
		t.Fatalf("status = %d", response.Code)
	}
	if location := response.Header().Get("Location"); location != "/forward/12/edit?source=bookmark" {
		t.Fatalf("location = %q", location)
	}
}

func TestServiceConflictDetection(t *testing.T) {
	s := newDNSTestServer(t)
	if err := s.initCertificateSchema(); err != nil {
		t.Fatal(err)
	}
	if err := s.initWebServerSchema(); err != nil {
		t.Fatal(err)
	}
	if err := s.initTCPListenerSchema(); err != nil {
		t.Fatal(err)
	}
	if _, err := s.CreateTCPListener("dns-conflict", "tcp", 15353, "127.0.0.1", 53); err != nil {
		t.Fatal(err)
	}
	if err := s.setServiceDesired("dns", true); err != nil {
		t.Fatal(err)
	}
	conflicts, err := s.configuredServiceConflicts("")
	if err != nil {
		t.Fatal(err)
	}
	if len(conflicts) != 1 || conflicts[0].Endpoint != "tcp:15353" {
		t.Fatalf("conflicts = %#v", conflicts)
	}
}

func TestServiceCoordinatorStartsAndStopsDNS(t *testing.T) {
	s := newDNSTestServer(t)
	if err := s.initCertificateSchema(); err != nil {
		t.Fatal(err)
	}
	if err := s.initWebServerSchema(); err != nil {
		t.Fatal(err)
	}
	if err := s.initTCPListenerSchema(); err != nil {
		t.Fatal(err)
	}
	settings, err := s.GetDNSSettings()
	if err != nil {
		t.Fatal(err)
	}
	settings.UDPAddr = "127.0.0.1:0"
	settings.TCPAddr = "127.0.0.1:0"
	settings.Upstreams = ""
	if err := s.UpdateDNSSettings(*settings); err != nil {
		t.Fatal(err)
	}

	runServiceAction := func(action string) *httptest.ResponseRecorder {
		form := url.Values{"action": {action}}
		request := httptest.NewRequest(http.MethodPost, "/services/dns/action", strings.NewReader(form.Encode()))
		request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		response := httptest.NewRecorder()
		s.serviceAction(response, request, "dns", "/dns")
		return response
	}

	if response := runServiceAction("start"); response.Code != http.StatusSeeOther {
		t.Fatalf("start status = %d, body = %s", response.Code, response.Body.String())
	}
	if status := s.dnsRuntime.Status(); !status.Running {
		t.Fatalf("DNS status after start = %#v", status)
	}
	if response := runServiceAction("stop"); response.Code != http.StatusSeeOther {
		t.Fatalf("stop status = %d, body = %s", response.Code, response.Body.String())
	}
	if status := s.dnsRuntime.Status(); status.Running {
		t.Fatalf("DNS status after stop = %#v", status)
	}
}

func TestDNSAndWebServerFormClasses(t *testing.T) {
	formTag := regexp.MustCompile(`<form\b[^>]*>`)
	inputTag := regexp.MustCompile(`<input\b[^>]*>`)
	selectTag := regexp.MustCompile(`<(?:select|textarea)\b[^>]*>`)
	hasFormClass := regexp.MustCompile(`class="[^"]*\bform\b[^"]*"`)
	hasInputClass := regexp.MustCompile(`class="[^"]*\binput\b[^"]*"`)

	var names []string
	for _, pattern := range []string{"acme/*.html", "dns/*.html", "webservers/*.html", "services/*.html", "forward/*.html"} {
		matches, err := fs.Glob(webfiles.Files, pattern)
		if err != nil {
			t.Fatal(err)
		}
		names = append(names, matches...)
	}
	for _, name := range names {
		data, err := fs.ReadFile(webfiles.Files, name)
		if err != nil {
			t.Fatal(err)
		}
		content := string(data)
		for _, tag := range formTag.FindAllString(content, -1) {
			if !hasFormClass.MatchString(tag) {
				t.Errorf("%s: form missing class=form: %s", name, strings.TrimSpace(tag))
			}
		}
		for _, tag := range inputTag.FindAllString(content, -1) {
			inlineChoice := strings.Contains(tag, `type="checkbox"`) || strings.Contains(tag, `type="radio"`)
			if !inlineChoice && !hasInputClass.MatchString(tag) {
				t.Errorf("%s: input missing class=input: %s", name, strings.TrimSpace(tag))
			}
			if !strings.Contains(tag, "type=") {
				t.Errorf("%s: input missing explicit type: %s", name, strings.TrimSpace(tag))
			}
		}
		for _, tag := range selectTag.FindAllString(content, -1) {
			if !hasInputClass.MatchString(tag) {
				t.Errorf("%s: select/textarea missing class=input: %s", name, strings.TrimSpace(tag))
			}
		}
	}
}

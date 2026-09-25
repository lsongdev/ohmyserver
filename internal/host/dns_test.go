package host

import (
	"bytes"
	"database/sql"
	"io"
	"net/http"
	"path/filepath"
	"testing"
	"time"

	dnsclient "github.com/lsongdev/dns-go/client"
	"github.com/lsongdev/dns-go/packet"
	"github.com/lsongdev/ohmyserver/config"
	dnsmodule "github.com/lsongdev/ohmyserver/modules/dns"
	dnsweb "github.com/lsongdev/ohmyserver/modules/dns/web"
	forwardmodule "github.com/lsongdev/ohmyserver/modules/forward"
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
	s := &Server{db: db, config: cfg}
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
	zone, err := s.DNSRepository().CreateZone(t.Context(), "Home.Test.", "local services", true)
	if err != nil {
		t.Fatal(err)
	}
	record := &dnsmodule.LocalRecord{
		ZoneID:  zone.ID,
		Name:    "router",
		Type:    "A",
		TTL:     120,
		Value:   "192.0.2.1",
		Enabled: true,
	}
	if err := s.DNSRepository().SaveRecord(t.Context(), record); err != nil {
		t.Fatal(err)
	}
	if record.ID == 0 {
		t.Fatal("record ID was not assigned")
	}

	settings, err := s.DNSRepository().GetSettings(t.Context())
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
	if err := s.DNSRepository().UpdateSettings(t.Context(), *settings); err != nil {
		t.Fatal(err)
	}
	cfg, enabled, err := dnsweb.BuildConfig(t.Context(), s.DNSRepository(), s.dnsListDirectory(), false)
	if err != nil {
		t.Fatal(err)
	}
	if !enabled || len(cfg.Domains) != 1 || len(cfg.Domains[0].Records) != 1 {
		t.Fatalf("unexpected DNS config: enabled=%v config=%#v", enabled, cfg)
	}

	if err := s.DNSRepository().DeleteZone(t.Context(), zone.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := s.DNSRepository().Record(t.Context(), record.ID); err != sql.ErrNoRows {
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
	server := &Server{db: db, config: &config.Config{}}
	if err := server.initDNSSchema(); err != nil {
		t.Fatal(err)
	}
	connection, err := server.DNSRepository().Get(t.Context(), 7)
	if err != nil {
		t.Fatal(err)
	}
	if connection.Name != "legacy-cloudflare" || connection.Type != "cloudflare" {
		t.Fatalf("connection = %#v", connection)
	}
}

func TestDNSRuntimeServesLocalRecordsOverUDPAndTCP(t *testing.T) {
	s := newDNSTestServer(t)
	zone, err := s.DNSRepository().CreateZone(t.Context(), "example.test", "", true)
	if err != nil {
		t.Fatal(err)
	}
	if err := s.DNSRepository().SaveRecord(t.Context(), &dnsmodule.LocalRecord{
		ZoneID:  zone.ID,
		Name:    "host",
		Type:    "A",
		TTL:     60,
		Value:   "192.0.2.25",
		Enabled: true,
	}); err != nil {
		t.Fatal(err)
	}
	settings, err := s.DNSRepository().GetSettings(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	settings.Enabled = true
	settings.UDPAddr = "127.0.0.1:0"
	settings.TCPAddr = "127.0.0.1:0"
	settings.DoHAddr = "127.0.0.1:0"
	settings.Upstreams = ""
	if err := s.DNSRepository().UpdateSettings(t.Context(), *settings); err != nil {
		t.Fatal(err)
	}
	if err := s.setServiceDesired("dns", true); err != nil {
		t.Fatal(err)
	}
	if err := s.reloadDNS(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = s.dnsService.Stop() })

	status := s.dnsLocalService(t.Context()).RuntimeStatus()
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

func TestServiceConflictDetection(t *testing.T) {
	s := newDNSTestServer(t)
	if err := s.ACMERepository().Initialize(t.Context()); err != nil {
		t.Fatal(err)
	}
	if err := s.initWebServerSchema(); err != nil {
		t.Fatal(err)
	}
	if err := s.initTCPListenerSchema(); err != nil {
		t.Fatal(err)
	}
	listener := &forwardmodule.Listener{Name: "dns-conflict", Protocol: "tcp", ListenPort: 15353, ForwardHost: "127.0.0.1", ForwardPort: 53}
	if err := s.ForwardRepository().Create(t.Context(), listener); err != nil {
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
	if err := s.ACMERepository().Initialize(t.Context()); err != nil {
		t.Fatal(err)
	}
	if err := s.initWebServerSchema(); err != nil {
		t.Fatal(err)
	}
	if err := s.initTCPListenerSchema(); err != nil {
		t.Fatal(err)
	}
	settings, err := s.DNSRepository().GetSettings(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	settings.UDPAddr = "127.0.0.1:0"
	settings.TCPAddr = "127.0.0.1:0"
	settings.Upstreams = ""
	if err := s.DNSRepository().UpdateSettings(t.Context(), *settings); err != nil {
		t.Fatal(err)
	}
	if err := s.DNSModuleService().DNSAction(t.Context(), "start"); err != nil {
		t.Fatalf("start: %v", err)
	}
	if status := s.dnsLocalService(t.Context()).RuntimeStatus(); !status.Running {
		t.Fatalf("DNS status after start = %#v", status)
	}
	if err := s.DNSModuleService().DNSAction(t.Context(), "stop"); err != nil {
		t.Fatalf("stop: %v", err)
	}
	if status := s.dnsLocalService(t.Context()).RuntimeStatus(); status.Running {
		t.Fatalf("DNS status after stop = %#v", status)
	}
}

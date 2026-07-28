package server

import (
	"database/sql"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"
	"time"

	"github.com/lsongdev/dhcp-go/dhcp4"
	"github.com/lsongdev/dhcp-go/dhcp4/options"
	ntpclient "github.com/lsongdev/ntp-go/ntp"
	"github.com/lsongdev/ohmyserver/config"
	_ "modernc.org/sqlite"
)

func newNetworkServicesTestServer(t *testing.T) *Server {
	t.Helper()
	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	db.SetMaxOpenConns(1)
	t.Cleanup(func() { _ = db.Close() })
	cfg := &config.Config{Listen: "127.0.0.1:9000"}
	cfg.Database.Path = filepath.Join(t.TempDir(), "ohmyserver.db")
	s := &Server{
		db: db, config: cfg,
		ntpRuntime: &NTPRuntime{}, dhcpRuntime: &DHCPRuntime{},
	}
	if err := s.initServicesSchema(); err != nil {
		t.Fatal(err)
	}
	if err := s.initNTPSchema(); err != nil {
		t.Fatal(err)
	}
	if err := s.initDHCPSchema(); err != nil {
		t.Fatal(err)
	}
	return s
}

func TestNTPRuntimeServesTimeAndStops(t *testing.T) {
	s := newNetworkServicesTestServer(t)
	settings, err := s.GetNTPSettings()
	if err != nil {
		t.Fatal(err)
	}
	settings.Enabled = true
	settings.ListenAddr = "127.0.0.1:0"
	if err := s.UpdateNTPSettings(*settings); err != nil {
		t.Fatal(err)
	}
	if err := s.reloadNTP(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = s.ntpRuntime.Stop() })

	status := s.ntpRuntime.Status()
	if !status.Running || status.ListenAddr == "" {
		t.Fatalf("unexpected NTP status: %#v", status)
	}
	client := ntpclient.NewClient(status.ListenAddr)
	client.Timeout = time.Second
	got, err := client.GetTime()
	if err != nil {
		t.Fatal(err)
	}
	if delta := time.Since(got); delta < -time.Second || delta > time.Second {
		t.Fatalf("NTP time differs from system time by %v", delta)
	}
	if err := s.ntpRuntime.Stop(); err != nil {
		t.Fatal(err)
	}
	if s.ntpRuntime.Status().Running {
		t.Fatal("NTP runtime remained running after Stop")
	}
}

type captureDHCPWriter struct {
	offer string
	ack   string
	nak   string
}

func (w *captureDHCPWriter) SendOffer(ip string, _ ...options.Option) {
	w.offer = ip
}

func (w *captureDHCPWriter) SendAck(ip string, _ ...options.Option) {
	w.ack = ip
}

func (w *captureDHCPWriter) SendNak(reason string, _ ...options.Option) {
	w.nak = reason
}

func (w *captureDHCPWriter) WriteResponse(_ *dhcp4.Message, _ ...options.Option) error {
	return nil
}

func TestDHCPStaticLeaseAndLeaseLifecycle(t *testing.T) {
	s := newNetworkServicesTestServer(t)
	settings, err := s.GetDHCPSettings()
	if err != nil {
		t.Fatal(err)
	}
	settings.ListenAddr = "127.0.0.1:0"
	settings.NTPServers = "192.168.1.1"
	if err := s.UpdateDHCPSettings(*settings); err != nil {
		t.Fatal(err)
	}
	if err := s.saveDHCPStaticLease(0, DHCPStaticLease{
		MAC: "02:00:00:00:00:10", IP: "192.168.1.110", Hostname: "printer",
	}); err != nil {
		t.Fatal(err)
	}
	handler, err := newEmbeddedDHCPHandler(s, *settings)
	if err != nil {
		t.Fatal(err)
	}
	discover := dhcp4.NewDiscoverMessage()
	discover.SetMacAddress("02:00:00:00:00:10")
	discover.SetHostName("printer")
	writer := &captureDHCPWriter{}
	handler.HandleDiscover(discover, writer)
	if writer.offer != "192.168.1.110" {
		t.Fatalf("offer = %q, want reserved address", writer.offer)
	}
	leases, err := s.GetAllDHCPLeases()
	if err != nil {
		t.Fatal(err)
	}
	if len(leases) != 1 || leases[0].IP != "192.168.1.110" || leases[0].State != "offered" {
		t.Fatalf("leases = %#v", leases)
	}

	settings.Enabled = true
	if err := s.UpdateDHCPSettings(*settings); err != nil {
		t.Fatal(err)
	}
	if err := s.reloadDHCP(); err != nil {
		t.Fatal(err)
	}
	status := s.dhcpRuntime.Status()
	if !status.Running || status.ListenAddr == "" {
		t.Fatalf("unexpected DHCP status: %#v", status)
	}
	if err := s.dhcpRuntime.Stop(); err != nil {
		t.Fatal(err)
	}
}

func TestNetworkServicePagesRender(t *testing.T) {
	s := newNetworkServicesTestServer(t)
	tests := []struct {
		name    string
		handler func(http.ResponseWriter, *http.Request)
		path    string
	}{
		{name: "NTP", handler: s.NTPView, path: "/ntp"},
		{name: "DHCP", handler: s.DHCPView, path: "/dhcp"},
		{name: "DHCP leases", handler: s.DHCPLeaseListView, path: "/dhcp/leases"},
		{name: "DHCP static leases", handler: s.DHCPStaticLeaseListView, path: "/dhcp/static-leases"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			req := httptest.NewRequest("GET", test.path, nil)
			rec := httptest.NewRecorder()
			test.handler(rec, req)
			if rec.Code != 200 {
				t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
			}
		})
	}
}

func TestDHCPReservationTableMigratesToStaticLeases(t *testing.T) {
	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	db.SetMaxOpenConns(1)
	if _, err := db.Exec(`
		CREATE TABLE dhcp_reservations (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			mac TEXT NOT NULL UNIQUE COLLATE NOCASE,
			ip TEXT NOT NULL UNIQUE,
			hostname TEXT NOT NULL DEFAULT '',
			description TEXT NOT NULL DEFAULT '',
			created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
			updated_at DATETIME DEFAULT CURRENT_TIMESTAMP
		);
		INSERT INTO dhcp_reservations (mac, ip, hostname, description)
		VALUES ('02:00:00:00:00:20', '192.168.1.120', 'nas', 'storage');
	`); err != nil {
		t.Fatal(err)
	}
	s := &Server{db: db}
	if err := s.initDHCPSchema(); err != nil {
		t.Fatal(err)
	}
	items, err := s.GetAllDHCPStaticLeases()
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 1 || items[0].Hostname != "nas" {
		t.Fatalf("static leases = %#v", items)
	}
	var count int
	if err := db.QueryRow(`
		SELECT COUNT(*) FROM sqlite_master
		WHERE type = 'table' AND name = 'dhcp_reservations'
	`).Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != 0 {
		t.Fatal("legacy reservations table was not removed")
	}
}

package host

import (
	"context"
	"database/sql"
	"errors"
	"path/filepath"
	"testing"
	"time"

	"github.com/lsongdev/dhcp-go/dhcp4"
	"github.com/lsongdev/dhcp-go/dhcp4/options"
	ntpclient "github.com/lsongdev/ntp-go/ntp"
	"github.com/lsongdev/ohmyserver/config"
	dhcpmodule "github.com/lsongdev/ohmyserver/modules/dhcp"
	dhcpweb "github.com/lsongdev/ohmyserver/modules/dhcp/web"
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
	settings, err := s.NTPRepository().Get(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	settings.Enabled = true
	settings.ListenAddr = "127.0.0.1:0"
	if err := s.NTPRepository().Update(t.Context(), *settings); err != nil {
		t.Fatal(err)
	}
	if err := s.reloadNTP(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = s.ntpModuleService().Close() })

	status := s.ntpModuleService().RuntimeStatus()
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
	if err := s.ntpModuleService().Stop(); err != nil {
		t.Fatal(err)
	}
	if s.ntpModuleService().RuntimeStatus().Running {
		t.Fatal("NTP runtime remained running after Stop")
	}
}

func TestNTPConfigurationHonorsCancelledContext(t *testing.T) {
	s := newNetworkServicesTestServer(t)
	before, err := s.NTPRepository().Get(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	updated := *before
	updated.ListenAddr = ":1123"
	if err := s.NTPModuleService().UpdateNTPConfiguration(ctx, updated); !errors.Is(err, context.Canceled) {
		t.Fatalf("update error=%v", err)
	}
	after, err := s.NTPRepository().Get(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	if after.ListenAddr != before.ListenAddr {
		t.Fatalf("cancelled update was persisted: before=%q after=%q", before.ListenAddr, after.ListenAddr)
	}
}

func TestDHCPConfigurationHonorsCancelledContext(t *testing.T) {
	s := newNetworkServicesTestServer(t)
	before, err := s.dhcpRepository().Settings(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	updated := *before
	updated.ListenAddr = ":1067"
	if err := (dhcpServiceAdapter{server: s}).UpdateDHCPConfiguration(ctx, updated); !errors.Is(err, context.Canceled) {
		t.Fatalf("update error=%v", err)
	}
	after, err := s.dhcpRepository().Settings(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	if after.ListenAddr != before.ListenAddr {
		t.Fatalf("cancelled update was persisted: before=%q after=%q", before.ListenAddr, after.ListenAddr)
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
	settings, err := s.dhcpRepository().Settings(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	settings.ListenAddr = "127.0.0.1:0"
	settings.NTPServers = "192.168.1.1"
	if err := s.dhcpRepository().UpdateSettings(t.Context(), *settings); err != nil {
		t.Fatal(err)
	}
	if err := s.dhcpModuleService().SaveReservation(t.Context(), 0, dhcpmodule.StaticLease{
		MAC: "02:00:00:00:00:10", IP: "192.168.1.110", Hostname: "printer",
	}); err != nil {
		t.Fatal(err)
	}
	handler, err := dhcpweb.NewPacketHandler(context.Background(), s.dhcpRepository(), *settings)
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
	leases, err := s.dhcpRepository().Leases(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	if len(leases) != 1 || leases[0].IP != "192.168.1.110" || leases[0].State != "offered" {
		t.Fatalf("leases = %#v", leases)
	}

	settings.Enabled = true
	if err := s.dhcpRepository().UpdateSettings(t.Context(), *settings); err != nil {
		t.Fatal(err)
	}
	if err := s.localDHCPModule().Start(t.Context()); err != nil {
		t.Fatal(err)
	}
	status := s.dhcpModuleService().Runtime().Status()
	if !status.Running || status.ListenAddr == "" {
		t.Fatalf("unexpected DHCP status: %#v", status)
	}
	if err := s.localDHCPModule().Close(); err != nil {
		t.Fatal(err)
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
	items, err := s.dhcpRepository().StaticLeases(t.Context())
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


func TestDHCPSettingsRemainAuthoritativeForHostState(t *testing.T) {
	s := newNetworkServicesTestServer(t)
	settings, err := s.dhcpRepository().Settings(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	settings.Enabled = true
	settings.ListenAddr = "127.0.0.1:9000"
	if err := s.dhcpRepository().UpdateSettings(t.Context(), *settings); err != nil {
		t.Fatal(err)
	}
	if err := s.setServiceDesired("dhcp", false); err != nil {
		t.Fatal(err)
	}
	for _, id := range []string{"web", "forward", "dns", "ntp"} {
		if err := s.setServiceDesired(id, false); err != nil {
			t.Fatal(err)
		}
	}

	conflicts, err := s.configuredServiceConflicts("")
	if err != nil {
		t.Fatal(err)
	}
	if len(conflicts) == 0 {
		t.Fatal("enabled DHCP settings were ignored because the host preference mirror was stale")
	}

	if err := s.ensureServicePreferences(); err != nil {
		t.Fatal(err)
	}
	preference, err := s.getServicePreference("dhcp", false)
	if err != nil {
		t.Fatal(err)
	}
	if !preference.Enabled {
		t.Fatal("DHCP host preference mirror was not synchronized from module settings")
	}
}

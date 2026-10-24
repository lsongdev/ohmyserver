package dns

import (
	"context"
	"crypto/tls"
	"database/sql"
	"path/filepath"
	"strings"
	"testing"

	acmeweb "github.com/lsongdev/acme-go/web"
	dnsapi "github.com/lsongdev/dns-go/api"
	"github.com/lsongdev/dns-go/config"
	dnsweb "github.com/lsongdev/dns-go/web"
	_ "modernc.org/sqlite"
)

func openDB(t *testing.T) *sql.DB {
	t.Helper()
	db, err := sql.Open("sqlite", filepath.Join(t.TempDir(), "ohmyserver.db")+"?_pragma=foreign_keys(1)&_pragma=busy_timeout(5000)")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	return db
}

// newDNS returns a DNS module in mode with one zone, example.com, whose
// records are given. It is not started: the adapters only use its data.
func newDNS(t *testing.T, mode string, records ...string) *dnsweb.DNS {
	t.Helper()
	srv := config.Server{Mode: mode, Listens: []config.ListenSpec{{Type: "udp", Addr: "127.0.0.1:0"}}}
	if mode == config.ModeForwarding {
		srv.Upstreams = []config.UpstreamSpec{{Type: "udp", Addr: "127.0.0.1:1"}}
	}
	d, err := dnsweb.New(context.Background(), openDB(t), dnsweb.Options{Server: srv, DataDir: t.TempDir()})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { d.Close() })
	err = d.Manager.Update(context.Background(), func(data *config.Data) error {
		data.Domains = []config.DomainSpec{{Domain: "example.com", Records: append([]string{
			"@ 3600 IN SOA ns1 hostmaster 1 7200 3600 1209600 300", "@ 3600 IN NS ns1",
		}, records...)}}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	return d
}

func lookup(t *testing.T, d *dnsweb.DNS, name string) []dnsapi.Record {
	t.Helper()
	records, err := d.Manager.Lookup(name)
	if err != nil {
		t.Fatal(err)
	}
	return records
}

func TestChallenges(t *testing.T) {
	ctx := context.Background()
	d := newDNS(t, config.ModeAuthoritative)
	c := Challenges{Manager: d.Manager}
	if !c.Covers("_acme-challenge.www.example.com") || c.Covers("_acme-challenge.example.org") {
		t.Fatal("scope does not follow the zones")
	}
	name := "_acme-challenge.www.example.com"
	if err := c.Present(ctx, name, "abc-DEF_123"); err != nil {
		t.Fatal(err)
	}
	if got := lookup(t, d, name); len(got) != 1 || got[0].Type != "TXT" || got[0].Data != `"abc-DEF_123"` {
		t.Fatalf("published %+v", got)
	}
	if err := c.CleanUp(ctx, name, "abc-DEF_123"); err != nil {
		t.Fatal(err)
	}
	if got := lookup(t, d, name); len(got) != 0 {
		t.Fatalf("left %+v", got)
	}

	// A forwarding resolver's zones are invisible to the CA.
	if (Challenges{Manager: newDNS(t, config.ModeForwarding).Manager}).Covers(name) {
		t.Fatal("forwarding zone offered for DNS-01")
	}
}

func TestAppRecords(t *testing.T) {
	ctx := context.Background()
	var notes []string
	log := func(s string) { notes = append(notes, s) }
	d := newDNS(t, config.ModeForwarding, "taken 300 IN A 192.0.2.9")
	a := &AppRecords{Manager: d.Manager, Address: "192.168.1.10"}

	a.Publish(ctx, "app.example.com", log)
	a.Publish(ctx, "app.example.com", log) // idempotent
	if got := lookup(t, d, "app.example.com"); len(got) != 1 || got[0].Type != "A" || got[0].Data != "192.168.1.10" {
		t.Fatalf("published %+v", got)
	}
	// Names with an operator's address records are left alone.
	a.Publish(ctx, "taken.example.com", log)
	if got := lookup(t, d, "taken.example.com"); len(got) != 1 || got[0].Data != "192.0.2.9" {
		t.Fatalf("operator record changed: %+v", got)
	}
	if !strings.Contains(strings.Join(notes, "\n"), "left unchanged") {
		t.Fatalf("notes = %q", notes)
	}
	// Domains outside every zone are not DNS's business.
	notes = nil
	a.Publish(ctx, "app.example.org", log)
	if len(notes) != 0 {
		t.Fatalf("notes for a foreign domain: %q", notes)
	}
	if err := a.Unpublish(ctx, "app.example.org"); err != nil {
		t.Fatal(err)
	}

	if err := a.Unpublish(ctx, "app.example.com"); err != nil {
		t.Fatal(err)
	}
	if got := lookup(t, d, "app.example.com"); len(got) != 0 {
		t.Fatalf("left %+v", got)
	}
	// Unpublishing never removes an operator's record.
	if err := (&AppRecords{Manager: d.Manager, Address: "192.0.2.9"}).Unpublish(ctx, "other.example.com"); err != nil {
		t.Fatal(err)
	}

	// Without an IP address there is nothing to point at.
	notes = nil
	(&AppRecords{Manager: d.Manager, Address: "localhost"}).Publish(ctx, "app.example.com", log)
	if len(notes) != 1 || !strings.Contains(notes[0], "not an IP address") || len(lookup(t, d, "app.example.com")) != 0 {
		t.Fatalf("notes = %q", notes)
	}
}

// A zone served to the internet never gets a private address.
func TestAppRecordsAuthoritativeSkipsPrivateAddresses(t *testing.T) {
	ctx := context.Background()
	d := newDNS(t, config.ModeAuthoritative)
	var notes []string
	(&AppRecords{Manager: d.Manager, Address: "192.168.1.10"}).Publish(ctx, "app.example.com", func(s string) { notes = append(notes, s) })
	if len(lookup(t, d, "app.example.com")) != 0 || len(notes) != 1 {
		t.Fatalf("private address published; notes = %q", notes)
	}
	(&AppRecords{Manager: d.Manager, Address: "2001:db8::10"}).Publish(ctx, "v6.example.com", func(string) {})
	if got := lookup(t, d, "v6.example.com"); len(got) != 1 || got[0].Type != "AAAA" {
		t.Fatalf("published %+v", got)
	}
}

func TestCertificatesWithoutAny(t *testing.T) {
	certs, err := acmeweb.New(context.Background(), openDB(t), acmeweb.Options{})
	if err != nil {
		t.Fatal(err)
	}
	defer certs.Close()
	get := Certificates(certs.Service)
	if _, err := get(&tls.ClientHelloInfo{ServerName: "dns.example.com"}); err == nil || !strings.Contains(err.Error(), "dns.example.com") {
		t.Fatalf("err = %v", err)
	}
}

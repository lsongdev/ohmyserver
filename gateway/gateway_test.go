package gateway

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"database/sql"
	"io"
	"math/big"
	"net/http"
	"net/http/httptest"
	"net/url"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	acmeweb "github.com/lsongdev/acme-go/web"
	"github.com/lsongdev/docker-go/apps"
	"github.com/lsongdev/yabber"
	yabberapi "github.com/lsongdev/yabber/api"
	_ "modernc.org/sqlite"
)

type fixture struct {
	gw     *yabber.Server
	api    *yabberapi.API
	certs  *acmeweb.ACME
	saved  int
	issued []tls.Certificate // what the stand-in certificate source serves
}

func newFixture(t *testing.T) *fixture {
	t.Helper()
	db, err := sql.Open("sqlite", filepath.Join(t.TempDir(), "ohmyserver.db")+"?_pragma=foreign_keys(1)")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	certs, err := acmeweb.New(context.Background(), db, acmeweb.Options{})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { certs.Close() })
	f := &fixture{certs: certs}
	// Like a real installation: plain HTTP and HTTPS listeners.
	gw, err := yabber.New(yabber.Config{
		Listeners: []yabber.Listener{{Name: "http", Address: "127.0.0.1:0"}, {Name: "https", Address: "127.0.0.1:0", TLS: true}},
		Sites: []yabber.Site{{Name: "mine", Hosts: []string{"mine.example.com"}, Listeners: []string{"http"},
			Handler: yabber.Handler{Respond: &yabber.Respond{Body: "operator"}}}},
	}, yabber.WithCertificateSource(yabber.CertificateSourceFunc(func() ([]tls.Certificate, error) { return f.issued, nil })),
		yabber.WithHTTP01Store(Challenges(certs.Service)))
	if err != nil {
		t.Fatal(err)
	}
	f.gw = gw
	f.api, err = yabberapi.New(gw, yabberapi.Options{Save: func(yabber.Config) error { f.saved++; return nil }})
	if err != nil {
		t.Fatal(err)
	}
	return f
}

// get sends a request for host through the gateway's http listener.
func (f *fixture) get(t *testing.T, host string) (int, string) {
	t.Helper()
	return f.getOn(t, "http", host)
}

// getOn sends a request for host through a gateway listener.
func (f *fixture) getOn(t *testing.T, listener, host string) (int, string) {
	t.Helper()
	handler, err := f.gw.Handler(listener)
	if err != nil {
		t.Fatal(err)
	}
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, httptest.NewRequest("GET", "http://"+host+"/", nil))
	return w.Code, w.Body.String()
}

func TestExposeRoutesDomainToApp(t *testing.T) {
	f := newFixture(t)
	app := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { io.WriteString(w, "app at "+r.Host) }))
	defer app.Close()
	u, _ := url.Parse(app.URL)
	port, _ := strconv.Atoi(u.Port())

	var notes []string
	exposer := &AppExposer{Gateway: f.api, Server: f.gw, Certs: f.certs.Service}
	err := exposer.Expose(context.Background(), apps.Exposure{ID: "notes", Domain: "notes.example.com", Port: port, Log: func(s string) { notes = append(notes, s) }})
	if err != nil {
		t.Fatal(err)
	}
	if status, body := f.get(t, "notes.example.com"); status != 200 || body != "app at notes.example.com" {
		t.Fatalf("proxied: %d %q", status, body)
	}
	if f.saved != 1 {
		t.Fatalf("gateway config saved %d times", f.saved)
	}
	// Without an ACME account the app is published over HTTP and the log
	// says how to get HTTPS.
	if !strings.Contains(strings.Join(notes, "\n"), "No ACME account") {
		t.Fatalf("notes = %q", notes)
	}

	// Exposing again (e.g. after a port change) replaces the route.
	if err := exposer.Expose(context.Background(), apps.Exposure{ID: "notes", Domain: "notes.example.com", Port: port}); err != nil {
		t.Fatal(err)
	}
	if n := len(f.gw.Config().Sites); n != 2 {
		t.Fatalf("sites = %d, want operator site + one app site", n)
	}

	// A domain an operator site already serves is refused.
	if err := exposer.Expose(context.Background(), apps.Exposure{ID: "clash", Domain: "mine.example.com", Port: port}); err == nil {
		t.Fatal("exposing over an operator site succeeded")
	}
	if _, body := f.get(t, "mine.example.com"); body != "operator" {
		t.Fatalf("operator site changed: %q", body)
	}

	if err := exposer.Unexpose(context.Background(), "notes"); err != nil {
		t.Fatal(err)
	}
	if status, _ := f.get(t, "notes.example.com"); status == 200 {
		t.Fatal("route still served after Unexpose")
	}
	if err := exposer.Unexpose(context.Background(), "notes"); err != nil {
		t.Fatalf("unexposing a missing route: %v", err)
	}
}

func TestCertificateAdapters(t *testing.T) {
	f := newFixture(t)
	certs, err := Certificates(f.certs.Service).Certificates()
	if err != nil || len(certs) != 0 {
		t.Fatalf("certificates = %d, %v", len(certs), err)
	}
	if _, ok := Challenges(f.certs.Service).GetHTTP01Token("example.com", "unknown"); ok {
		t.Fatal("unknown token answered")
	}
}

// selfSigned returns a certificate for host, standing in for one the
// certificate manager issued.
func selfSigned(t *testing.T, host string) tls.Certificate {
	t.Helper()
	key, _ := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	template := &x509.Certificate{
		SerialNumber: big.NewInt(1), Subject: pkix.Name{CommonName: host}, DNSNames: []string{host},
		NotBefore: time.Now().Add(-time.Hour), NotAfter: time.Now().Add(90 * 24 * time.Hour),
	}
	der, err := x509.CreateCertificate(rand.Reader, template, template, &key.PublicKey, key)
	if err != nil {
		t.Fatal(err)
	}
	leaf, _ := x509.ParseCertificate(der)
	return tls.Certificate{Certificate: [][]byte{der}, PrivateKey: key, Leaf: leaf}
}

// Publishing a domain before its certificate exists must not fail: the
// gateway refuses TLS hosts without a certificate, so the site starts on
// plain HTTP and moves onto HTTPS once the certificate is issued.
func TestExposeBeforeCertificateThenReconcile(t *testing.T) {
	f := newFixture(t)
	app := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { io.WriteString(w, "app") }))
	defer app.Close()
	u, _ := url.Parse(app.URL)
	port, _ := strconv.Atoi(u.Port())
	issued := false
	exposer := &AppExposer{Gateway: f.api, Server: f.gw, Certs: f.certs.Service,
		Covered: func(context.Context, string) bool { return issued }}

	var notes []string
	if err := exposer.Expose(context.Background(), apps.Exposure{ID: "tools", Domain: "tools.example.com", Port: port, Log: func(s string) { notes = append(notes, s) }}); err != nil {
		t.Fatal(err)
	}
	if status, body := f.getOn(t, "http", "tools.example.com"); status != 200 || body != "app" {
		t.Fatalf("http before certificate: %d %q", status, body)
	}
	if status, _ := f.getOn(t, "https", "tools.example.com"); status == 200 {
		t.Fatal("served over HTTPS without a certificate")
	}
	if !strings.Contains(strings.Join(notes, "\n"), "HTTPS starts once its certificate is issued") {
		t.Fatalf("notes = %q", notes)
	}
	if n, err := exposer.Reconcile(context.Background()); n != 0 || err != nil {
		t.Fatalf("reconcile before certificate: %d, %v", n, err)
	}

	// The certificate manager issues the certificate.
	f.issued = []tls.Certificate{selfSigned(t, "tools.example.com")}
	issued = true
	if n, err := exposer.Reconcile(context.Background()); n != 1 || err != nil {
		t.Fatalf("reconcile after certificate: %d, %v", n, err)
	}
	if status, body := f.getOn(t, "https", "tools.example.com"); status != 200 || body != "app" {
		t.Fatalf("https after certificate: %d %q", status, body)
	}
	// Plain HTTP now redirects to HTTPS.
	if status, _ := f.getOn(t, "http", "tools.example.com"); status != http.StatusPermanentRedirect && status != http.StatusMovedPermanently && status != http.StatusFound {
		t.Fatalf("http after certificate: %d", status)
	}
	if n, _ := exposer.Reconcile(context.Background()); n != 0 {
		t.Fatalf("second reconcile moved %d sites", n)
	}
}

package gateway

import (
	"context"
	"database/sql"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	acmeweb "github.com/lsongdev/acme-go/web"
	"github.com/lsongdev/docker-go/apps"
	"github.com/lsongdev/yabber"
	yabberapi "github.com/lsongdev/yabber/api"
	_ "modernc.org/sqlite"
)

type fixture struct {
	gw    *yabber.Server
	api   *yabberapi.API
	certs *acmeweb.ACME
	saved int
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
	gw, err := yabber.New(yabber.Config{
		Listeners: []yabber.Listener{{Name: "http", Address: "127.0.0.1:0"}},
		Sites:     []yabber.Site{{Name: "mine", Hosts: []string{"mine.example.com"}, Handler: yabber.Handler{Respond: &yabber.Respond{Body: "operator"}}}},
	}, yabber.WithCertificateSource(Certificates(certs.Service)), yabber.WithHTTP01Store(Challenges(certs.Service)))
	if err != nil {
		t.Fatal(err)
	}
	f := &fixture{gw: gw, certs: certs}
	f.api, err = yabberapi.New(gw, yabberapi.Options{Save: func(yabber.Config) error { f.saved++; return nil }})
	if err != nil {
		t.Fatal(err)
	}
	return f
}

// get sends a request for host through the gateway's http listener.
func (f *fixture) get(t *testing.T, host string) (int, string) {
	t.Helper()
	handler, err := f.gw.Handler("http")
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
	exposer := &AppExposer{Gateway: f.api, Certs: f.certs.Service}
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

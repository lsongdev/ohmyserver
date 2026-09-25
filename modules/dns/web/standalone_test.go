package web

import (
	"context"
	"database/sql"
	dnsruntime "github.com/lsongdev/ohmyserver/modules/dns/runtime"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	_ "modernc.org/sqlite"
)

func standaloneTestRepository(t *testing.T) *Repository {
	t.Helper()
	db, err := sql.Open("sqlite", filepath.Join(t.TempDir(), "dns.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	db.SetMaxOpenConns(1)
	repo := NewRepository(db)
	if err := repo.InitializeModule(context.Background()); err != nil {
		t.Fatal(err)
	}
	return repo
}

func TestStandaloneHandlerServesPageManifestAssetsAndMountedAPI(t *testing.T) {
	repo := standaloneTestRepository(t)
	service := &apiService{repo: repo}
	handler := New(repo, service, t.TempDir()).StandaloneHandler()
	for path, contains := range map[string]string{
		"/":                       "api-base",
		"/v1/dns/manifest.json":   "assets/module.js",
		"/v1/dns/assets/index.js": "lsong.org/scripts/react/index.js",
		"/v1/dns/assets/dns.css":  ".dns-standalone-header",
		"/v1/dns/status":          `"state":"running"`,
	} {
		response := httptest.NewRecorder()
		handler.ServeHTTP(response, httptest.NewRequest(http.MethodGet, path, nil))
		if response.Code != http.StatusOK || !strings.Contains(response.Body.String(), contains) {
			t.Errorf("GET %s: status=%d body=%s", path, response.Code, response.Body.String())
		}
	}
}

func TestLocalServiceStartsAndStopsItsDNSRuntime(t *testing.T) {
	repo := standaloneTestRepository(t)
	settings, err := repo.GetSettings(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	settings.UDPAddr, settings.TCPAddr, settings.DoTAddr, settings.DoHAddr = "127.0.0.1:0", "", "", ""
	if err := repo.UpdateSettings(context.Background(), *settings); err != nil {
		t.Fatal(err)
	}
	service := NewLocalService(context.Background(), repo, t.TempDir())
	t.Cleanup(func() { _ = service.Close() })
	if err := service.DNSAction(context.Background(), "start"); err != nil {
		t.Fatal(err)
	}
	status, err := service.DNSRuntimeStatus(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if !statusRunning(status) {
		t.Fatalf("DNS did not start: %#v", status)
	}
	if err := service.DNSAction(context.Background(), "stop"); err != nil {
		t.Fatal(err)
	}
	status, err = service.DNSRuntimeStatus(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if statusRunning(status) {
		t.Fatalf("DNS still running after stop: %#v", status)
	}
}

func statusRunning(value any) bool {
	state, ok := value.(map[string]any)
	if !ok {
		return false
	}
	runtimeStatus, ok := state["runtime"].(dnsruntime.DNSRuntimeStatus)
	return ok && runtimeStatus.Running
}

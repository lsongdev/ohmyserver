package host

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/json"
	"encoding/pem"
	"fmt"
	"math/big"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"os/signal"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/lsongdev/ohmyserver/config"
)

// TestBrowserSmokeHarness exposes a disposable, unauthenticated app instance
// for manual/automated browser acceptance. It is opt-in and never uses the
// operator's configured database.
func TestBrowserSmokeHarness(t *testing.T) {
	if os.Getenv("OHMYSERVER_BROWSER_SMOKE") != "1" {
		t.Skip("set OHMYSERVER_BROWSER_SMOKE=1 to run the disposable browser test server")
	}
	addr := os.Getenv("OHMYSERVER_BROWSER_SMOKE_ADDR")
	if addr == "" {
		addr = "127.0.0.1:9876"
	}
	cfg := &config.Config{Listen: addr, CollectionInterval: 3600}
	cfg.Database.Driver = "sqlite"
	cfg.Database.Path = filepath.Join(t.TempDir(), "browser-smoke.db")
	instance, err := NewServer(cfg)
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		if err := instance.Close(); err != nil {
			t.Errorf("close browser smoke server: %v", err)
		}
	}()
	router, err := NewHTTPRouter(instance)
	if err != nil {
		t.Fatal(err)
	}
	listener, err := net.Listen("tcp", addr)
	if err != nil {
		t.Fatal(err)
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	fmt.Printf("BROWSER_SMOKE_URL=http://%s/\n", listener.Addr())
	if err := serveHTTP(ctx, listener, loggingMiddleware(router)); err != nil {
		t.Fatal(err)
	}
}

func TestServeHTTPGracefullyShutsDownOnContextCancellation(t *testing.T) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	served := make(chan struct{}, 1)
	result := make(chan error, 1)
	go func() {
		result <- serveHTTP(ctx, listener, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			served <- struct{}{}
			w.WriteHeader(http.StatusNoContent)
		}))
	}()

	response, err := http.Get("http://" + listener.Addr().String())
	if err != nil {
		t.Fatalf("request local HTTP server: %v", err)
	}
	_ = response.Body.Close()
	select {
	case <-served:
	case <-time.After(2 * time.Second):
		t.Fatal("HTTP handler was not reached")
	}
	cancel()
	select {
	case err := <-result:
		if err != nil {
			t.Fatalf("serveHTTP after graceful shutdown: %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("HTTP server did not stop after context cancellation")
	}
	if _, err := http.Get("http://" + listener.Addr().String()); err == nil {
		t.Fatal("HTTP server still accepts requests after shutdown")
	}
}

func TestShellRouterServesSPAAndModuleAssets(t *testing.T) {
	router, err := NewHTTPRouter(&Server{})
	if err != nil {
		t.Fatal(err)
	}
	for _, path := range []string{"/", "/frontend/app/modules.js"} {
		response := httptest.NewRecorder()
		router.ServeHTTP(response, httptest.NewRequest(http.MethodGet, path, nil))
		if response.Code != http.StatusOK {
			t.Errorf("GET %s: status=%d body=%s", path, response.Code, response.Body.String())
		}
	}
}

func TestHTTPBasePathMountsShellAssetsAndFeatureAPIs(t *testing.T) {
	cfg := &config.Config{Listen: "127.0.0.1:0", CollectionInterval: 3600}
	cfg.AccessKey = "test-access-key"
	cfg.BasePath = "/panel"
	cfg.Database.Driver = "sqlite"
	cfg.Database.Path = filepath.Join(t.TempDir(), "base-path.db")
	instance, err := NewServer(cfg)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := instance.Close(); err != nil {
			t.Errorf("close isolated test server: %v", err)
		}
	})

	router, err := NewHTTPRouterAtBase(instance, "/panel")
	if err != nil {
		t.Fatal(err)
	}
	unauthorized := httptest.NewRecorder()
	router.ServeHTTP(unauthorized, httptest.NewRequest(http.MethodGet, "/panel/v1/modules", nil))
	if unauthorized.Code != http.StatusSeeOther || unauthorized.Header().Get("Location") != "/panel/login" {
		t.Fatalf("unauthenticated module index: status=%d location=%q", unauthorized.Code, unauthorized.Header().Get("Location"))
	}
	login := httptest.NewRecorder()
	router.ServeHTTP(login, httptest.NewRequest(http.MethodGet, "/panel/login", nil))
	if login.Code != http.StatusOK || !strings.Contains(login.Body.String(), "访问密钥") {
		t.Fatalf("GET /panel/login: status=%d body=%s", login.Code, login.Body.String())
	}
	loginRequest := httptest.NewRequest(http.MethodPost, "/panel/login", strings.NewReader("access_key=test-access-key"))
	loginRequest.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	login = httptest.NewRecorder()
	router.ServeHTTP(login, loginRequest)
	if login.Code != http.StatusSeeOther || login.Header().Get("Location") != "/panel/" || len(login.Result().Cookies()) != 1 {
		t.Fatalf("POST /panel/login: status=%d location=%q cookies=%v", login.Code, login.Header().Get("Location"), login.Result().Cookies())
	}
	authCookie := login.Result().Cookies()[0]
	for _, test := range []struct {
		path string
		want string
	}{
		{path: "/panel/", want: `<base href="/panel/">`},
		{path: "/panel/", want: `"apiBase":"/panel/v1"`},
		{path: "/panel/frontend/app/modules.js", want: "discoverModules"},
		{path: "/panel/v1/modules", want: `"modules"`},
		{path: "/panel/v1/docker/manifest.json", want: `"id": "docker"`},
		{path: "/panel/v1/docker/assets/module.js", want: "createAPI"},
	} {
		response := httptest.NewRecorder()
		request := httptest.NewRequest(http.MethodGet, test.path, nil)
		request.AddCookie(authCookie)
		router.ServeHTTP(response, request)
		if response.Code != http.StatusOK || !strings.Contains(response.Body.String(), test.want) {
			t.Errorf("GET %s: status=%d body=%s", test.path, response.Code, response.Body.String())
		}
	}
	for _, path := range []string{"/", "/v1/widgets/widgets", "/panelx/frontend/app/modules.js"} {
		response := httptest.NewRecorder()
		router.ServeHTTP(response, httptest.NewRequest(http.MethodGet, path, nil))
		if response.Code != http.StatusNotFound {
			t.Errorf("GET outside base path %s: status=%d body=%s", path, response.Code, response.Body.String())
		}
	}
	response := httptest.NewRecorder()
	router.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/panel", nil))
	if response.Code != http.StatusPermanentRedirect || response.Header().Get("Location") != "/panel/" {
		t.Fatalf("base path canonical redirect: status=%d location=%q", response.Code, response.Header().Get("Location"))
	}
	response = httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodHead, "/panel/", nil)
	request.AddCookie(authCookie)
	router.ServeHTTP(response, request)
	if response.Code != http.StatusOK || response.Body.Len() != 0 || response.Header().Get("Content-Length") == "" {
		t.Fatalf("HEAD base index: status=%d content-length=%q body=%d", response.Code, response.Header().Get("Content-Length"), response.Body.Len())
	}
}

func TestNormalizeHTTPBasePathRejectsUnsafeOrAmbiguousValues(t *testing.T) {
	for _, invalid := range []string{"panel", "/panel/../admin", "/panel?x=1", "/panel#fragment", "/panel//admin", "/panel%2fadmin"} {
		if _, err := normalizeHTTPBasePath(invalid); err == nil {
			t.Errorf("normalizeHTTPBasePath(%q) accepted invalid input", invalid)
		}
	}
	for _, valid := range []string{"", "/", "/panel", "/panel/", "/ops.v1/manage_1"} {
		if _, err := normalizeHTTPBasePath(valid); err != nil {
			t.Errorf("normalizeHTTPBasePath(%q): %v", valid, err)
		}
	}
}

func TestNewServerMountsFrontendAndFeatureAPIsWithIsolatedDatabase(t *testing.T) {
	cfg := &config.Config{Listen: "127.0.0.1:0", CollectionInterval: 3600}
	cfg.Database.Driver = "sqlite"
	cfg.Database.Path = filepath.Join(t.TempDir(), "integration.db")
	instance, err := NewServer(cfg)
	if err != nil {
		t.Fatal(err)
	}
	defaultListeners, err := instance.WebServerRepository().ListListeners(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	if len(defaultListeners) != 1 {
		t.Fatalf("default web listener count = %d, want 1", len(defaultListeners))
	}
	defaultListeners[0].Address = freeLoopbackAddress(t)
	if err := instance.WebServerRepository().UpdateListener(t.Context(), defaultListeners[0]); err != nil {
		t.Fatalf("move default listener to an ephemeral loopback port: %v", err)
	}
	t.Cleanup(func() {
		if err := instance.Close(); err != nil {
			t.Errorf("close isolated test server: %v", err)
		}
	})

	router, err := NewHTTPRouter(instance)
	if err != nil {
		t.Fatal(err)
	}
	for _, test := range []struct {
		path string
		want string
	}{
		{path: "/", want: "<div id=\"app\"></div>"},
		{path: "/frontend/app/modules.js", want: "discoverModules"},
		{path: "/v1/modules", want: `"modules"`},
		{path: "/v1/docker/manifest.json", want: `"id": "docker"`},
		{path: "/v1/docker/assets/docker.css", want: ".app-card"},
		{path: "/v1/dashboard/overview", want: `"system":{"hostname"`},
		{path: "/v1/acme/providers", want: `"data"`},
		{path: "/v1/acme/certificates", want: `"data"`},
		{path: "/v1/dns/settings", want: `"data"`},
		{path: "/v1/dns/providers", want: `"data"`},
		{path: "/v1/dns/zones", want: `"data"`},
		{path: "/v1/webserver/listeners", want: `"data"`},
		{path: "/v1/webserver/hosts", want: `"data"`},
		{path: "/v1/webserver/policies", want: `"data"`},
		{path: "/v1/webserver/certificates", want: `"data"`},
	} {
		response := httptest.NewRecorder()
		router.ServeHTTP(response, httptest.NewRequest(http.MethodGet, test.path, nil))
		if response.Code != http.StatusOK || !strings.Contains(response.Body.String(), test.want) {
			t.Errorf("GET %s: status=%d body=%s", test.path, response.Code, response.Body.String())
		}
	}

	call := func(method, path, body string) *httptest.ResponseRecorder {
		t.Helper()
		request := httptest.NewRequest(method, path, strings.NewReader(body))
		if body != "" {
			request.Header.Set("Content-Type", "application/json")
		}
		response := httptest.NewRecorder()
		router.ServeHTTP(response, request)
		return response
	}
	resourceID := func(response *httptest.ResponseRecorder) int64 {
		t.Helper()
		var payload struct {
			Data struct {
				ID int64 `json:"id"`
			} `json:"data"`
		}
		if err := json.Unmarshal(response.Body.Bytes(), &payload); err != nil || payload.Data.ID < 1 {
			t.Fatalf("decode created resource ID: id=%d err=%v body=%s", payload.Data.ID, err, response.Body.String())
		}
		return payload.Data.ID
	}
	certificatePEM, privateKeyPEM := newTestTLSCertificate(t, "integration.invalid")
	certificate, err := instance.ACMERepository().Create(t.Context(), "integration.invalid", "", string(privateKeyPEM))
	if err != nil {
		t.Fatal(err)
	}
	if err := instance.ACMERepository().Update(t.Context(), certificate.ID, 0, string(certificatePEM), "active"); err != nil {
		t.Fatal(err)
	}
	acmeList := call(http.MethodGet, "/v1/acme/certificates", "")
	if acmeList.Code != http.StatusOK || !strings.Contains(acmeList.Body.String(), `"common_name":"integration.invalid"`) || strings.Contains(acmeList.Body.String(), "PRIVATE KEY") {
		t.Fatalf("ACME certificate summary: status=%d body=%s", acmeList.Code, acmeList.Body.String())
	}
	webCertificateList := call(http.MethodGet, "/v1/webserver/certificates", "")
	if webCertificateList.Code != http.StatusOK || !strings.Contains(webCertificateList.Body.String(), `"domains":"integration.invalid"`) || strings.Contains(webCertificateList.Body.String(), "PRIVATE KEY") {
		t.Fatalf("Web Server certificate choices: status=%d body=%s", webCertificateList.Code, webCertificateList.Body.String())
	}
	tlsAddress := freeLoopbackAddress(t)
	tlsListener := call(http.MethodPost, "/v1/webserver/listeners", fmt.Sprintf(`{"name":"integration-tls","address":%q,"protocol":"https","enabled":true,"certificate_id":%d}`, tlsAddress, certificate.ID))
	if tlsListener.Code != http.StatusCreated {
		t.Fatalf("create HTTPS listener using ACME certificate: status=%d body=%s", tlsListener.Code, tlsListener.Body.String())
	}
	tlsListenerID := resourceID(tlsListener)
	listenersWithTLS := call(http.MethodGet, "/v1/webserver/listeners", "")
	if listenersWithTLS.Code != http.StatusOK || !strings.Contains(listenersWithTLS.Body.String(), `"certificate_name":"integration.invalid"`) || strings.Contains(listenersWithTLS.Body.String(), "PRIVATE KEY") {
		t.Fatalf("HTTPS listener certificate selection: status=%d body=%s", listenersWithTLS.Code, listenersWithTLS.Body.String())
	}
	if response := call(http.MethodDelete, "/v1/webserver/listeners/"+strconv.FormatInt(tlsListenerID, 10), ""); response.Code != http.StatusNoContent {
		t.Fatalf("delete integration HTTPS listener: status=%d body=%s", response.Code, response.Body.String())
	}
	policy := call(http.MethodPost, "/v1/webserver/policies", `{"name":"integration-policy","max_body_bytes":4096,"concurrency_max":2}`)
	if policy.Code != http.StatusCreated {
		t.Fatalf("create policy: status=%d body=%s", policy.Code, policy.Body.String())
	}
	policyID := resourceID(policy)
	policyPath := "/v1/webserver/policies/" + strconv.FormatInt(policyID, 10)
	listener := call(http.MethodPost, "/v1/webserver/listeners", `{"name":"integration-listener","address":"127.0.0.1:0","protocol":"http","enabled":false,"policy_ids":[`+strconv.FormatInt(policyID, 10)+`]}`)
	if listener.Code != http.StatusCreated {
		t.Fatalf("create disabled listener: status=%d body=%s", listener.Code, listener.Body.String())
	}
	listenerID := resourceID(listener)
	listenerPath := "/v1/webserver/listeners/" + strconv.FormatInt(listenerID, 10)
	created := call(http.MethodPost, "/v1/webserver/hosts", `{"name":"integration","hostnames":"integration.invalid","enabled":false,"listener_ids":[`+strconv.FormatInt(listenerID, 10)+`],"policy_ids":[`+strconv.FormatInt(policyID, 10)+`]}`)
	if created.Code != http.StatusCreated {
		t.Fatalf("create disabled host: status=%d body=%s", created.Code, created.Body.String())
	}
	hostID := resourceID(created)
	hostPath := "/v1/webserver/hosts/" + strconv.FormatInt(hostID, 10)
	route := call(http.MethodPost, "/v1/webserver/hosts/"+strconv.FormatInt(hostID, 10)+"/routes", `{"name":"health","path":"/health","handler_type":"static_response","response_status":200,"response_body":"ok","enabled":false,"policy_ids":[`+strconv.FormatInt(policyID, 10)+`]}`)
	if route.Code != http.StatusCreated {
		t.Fatalf("create disabled route: status=%d body=%s", route.Code, route.Body.String())
	}
	routeID := resourceID(route)
	routePath := "/v1/webserver/routes/" + strconv.FormatInt(routeID, 10)
	listed := call(http.MethodGet, "/v1/webserver/hosts", "")
	if listed.Code != http.StatusOK || !strings.Contains(listed.Body.String(), `"hostnames":"integration.invalid"`) || !strings.Contains(listed.Body.String(), `"listener_names":["integration-listener"]`) || !strings.Contains(listed.Body.String(), `"policy_names":["integration-policy"]`) || !strings.Contains(listed.Body.String(), `"name":"health"`) {
		t.Fatalf("list created host: status=%d body=%s", listed.Code, listed.Body.String())
	}
	updatedRoute := call(http.MethodPut, routePath, `{"name":"health","path":"/health","handler_type":"static_response","response_status":200,"response_body":"updated","enabled":false,"policy_ids":[`+strconv.FormatInt(policyID, 10)+`]}`)
	if updatedRoute.Code != http.StatusNoContent {
		t.Fatalf("update route: status=%d body=%s", updatedRoute.Code, updatedRoute.Body.String())
	}
	updatedPolicy := call(http.MethodPut, policyPath, `{"name":"integration-policy","max_body_bytes":8192,"concurrency_max":4}`)
	if updatedPolicy.Code != http.StatusNoContent {
		t.Fatalf("update policy: status=%d body=%s", updatedPolicy.Code, updatedPolicy.Body.String())
	}
	updatedListener := call(http.MethodPut, listenerPath, `{"name":"integration-listener","address":"127.0.0.1:1","protocol":"http","enabled":false,"policy_ids":[`+strconv.FormatInt(policyID, 10)+`]}`)
	if updatedListener.Code != http.StatusNoContent {
		t.Fatalf("update listener: status=%d body=%s", updatedListener.Code, updatedListener.Body.String())
	}
	updated := call(http.MethodPut, hostPath, `{"name":"integration","hostnames":"updated.invalid","enabled":false,"listener_ids":[`+strconv.FormatInt(listenerID, 10)+`],"policy_ids":[`+strconv.FormatInt(policyID, 10)+`]}`)
	if updated.Code != http.StatusNoContent {
		t.Fatalf("update disabled host: status=%d body=%s", updated.Code, updated.Body.String())
	}
	listed = call(http.MethodGet, "/v1/webserver/hosts", "")
	if listed.Code != http.StatusOK || !strings.Contains(listed.Body.String(), `"hostnames":"updated.invalid"`) || !strings.Contains(listed.Body.String(), `"response_body":"updated"`) {
		t.Fatalf("list updated host: status=%d body=%s", listed.Code, listed.Body.String())
	}
	deletedRoute := call(http.MethodDelete, routePath, "")
	if deletedRoute.Code != http.StatusNoContent {
		t.Fatalf("delete route: status=%d body=%s", deletedRoute.Code, deletedRoute.Body.String())
	}
	deleted := call(http.MethodDelete, hostPath, "")
	if deleted.Code != http.StatusNoContent {
		t.Fatalf("delete host: status=%d body=%s", deleted.Code, deleted.Body.String())
	}
	listed = call(http.MethodGet, "/v1/webserver/hosts", "")
	if listed.Code != http.StatusOK || strings.Contains(listed.Body.String(), "integration.invalid") || strings.Contains(listed.Body.String(), "updated.invalid") {
		t.Fatalf("deleted host remains visible: status=%d body=%s", listed.Code, listed.Body.String())
	}
	if response := call(http.MethodDelete, listenerPath, ""); response.Code != http.StatusNoContent {
		t.Fatalf("delete listener: status=%d body=%s", response.Code, response.Body.String())
	}
	if response := call(http.MethodDelete, policyPath, ""); response.Code != http.StatusNoContent {
		t.Fatalf("delete policy: status=%d body=%s", response.Code, response.Body.String())
	}
}

func newTestTLSCertificate(t *testing.T, hostname string) ([]byte, []byte) {
	t.Helper()
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now()
	template := &x509.Certificate{
		SerialNumber:          big.NewInt(1),
		Subject:               pkix.Name{CommonName: hostname},
		DNSNames:              []string{hostname},
		NotBefore:             now.Add(-time.Minute),
		NotAfter:              now.Add(24 * time.Hour),
		KeyUsage:              x509.KeyUsageDigitalSignature,
		ExtKeyUsage:           []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
		BasicConstraintsValid: true,
	}
	der, err := x509.CreateCertificate(rand.Reader, template, template, &key.PublicKey, key)
	if err != nil {
		t.Fatal(err)
	}
	keyDER, err := x509.MarshalPKCS8PrivateKey(key)
	if err != nil {
		t.Fatal(err)
	}
	return pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der}), pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: keyDER})
}

func freeLoopbackAddress(t *testing.T) string {
	t.Helper()
	listener, err := net.Listen("tcp4", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	address := listener.Addr().String()
	if err := listener.Close(); err != nil {
		t.Fatal(err)
	}
	return address
}

func TestShellRouterDoesNotKeepLegacyManagementRoutes(t *testing.T) {
	router, err := NewHTTPRouter(&Server{})
	if err != nil {
		t.Fatal(err)
	}
	for _, path := range []string{"/docker", "/dns/zones", "/web", "/openwrt/1", "/acme/certificates"} {
		response := httptest.NewRecorder()
		router.ServeHTTP(response, httptest.NewRequest(http.MethodGet, path, nil))
		if response.Code != http.StatusNotFound {
			t.Errorf("legacy UI route %s: status=%d location=%q body=%s", path, response.Code, response.Header().Get("Location"), response.Body.String())
		}
	}
	response := httptest.NewRecorder()
	router.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/v1/missing/resource", nil))
	if response.Code != http.StatusNotFound || response.Header().Get("Content-Type") != "application/json; charset=utf-8" || !strings.Contains(response.Body.String(), "api_module_not_found") {
		t.Fatalf("unknown API module: status=%d content-type=%q body=%s", response.Code, response.Header().Get("Content-Type"), response.Body.String())
	}
}

package esxi_test

import (
	"context"
	"database/sql"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	esxi "github.com/lsongdev/ohmyserver/modules/esxi"
	esxiweb "github.com/lsongdev/ohmyserver/modules/esxi/web"
	_ "modernc.org/sqlite"
)

type apiService struct {
	esxi.Service
	created esxi.Input
	updated esxi.Input
}

func (*apiService) ListESXi(context.Context) ([]esxi.DTO, error) {
	return []esxi.DTO{{ID: 3, Name: "host", Host: "192.0.2.3", Username: "root", HasCredential: true}}, nil
}
func (s *apiService) CreateESXi(_ context.Context, in esxi.Input) (esxi.DTO, error) {
	s.created = in
	return esxi.DTO{ID: 3, Name: in.Name, Host: in.Host, Username: in.Username, Port: in.Port, HasCredential: true}, nil
}
func (*apiService) GetESXi(context.Context, int64) (esxi.DTO, error) {
	return esxi.DTO{ID: 3, Name: "host", Host: "192.0.2.3", Username: "root", HasCredential: true}, nil
}
func (s *apiService) UpdateESXi(_ context.Context, _ int64, in esxi.Input) (esxi.DTO, error) {
	s.updated = in
	return esxi.DTO{ID: 3, Name: in.Name, Host: in.Host, Username: in.Username, HasCredential: true}, nil
}
func (*apiService) DeleteESXi(context.Context, int64) error { return nil }
func (*apiService) TestESXi(context.Context, int64) error   { return nil }
func (*apiService) ESXiInventory(context.Context, int64) (esxi.Inventory, error) {
	return esxi.Inventory{VMs: []any{}, Hosts: []any{}}, nil
}

func request(t *testing.T, handler http.Handler, method, path, body string) *httptest.ResponseRecorder {
	t.Helper()
	r := httptest.NewRequest(method, path, strings.NewReader(body))
	if body != "" {
		r.Header.Set("Content-Type", "application/json")
	}
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, r)
	return w
}

func TestESXiCredentialsAndInventoryAPI(t *testing.T) {
	service := &apiService{}
	handler := esxiweb.New(service).Handler()
	created := request(t, handler, http.MethodPost, "/servers", `{"name":"host","host":"192.0.2.3","username":"root","password":"secret"}`)
	if created.Code != http.StatusCreated || service.created.Password == nil || *service.created.Password != "secret" || service.created.Port != 0 {
		t.Fatalf("create status=%d input=%+v body=%s", created.Code, service.created, created.Body.String())
	}
	if strings.Contains(created.Body.String(), "secret") || strings.Contains(created.Body.String(), `"password"`) {
		t.Fatalf("response leaked credential: %s", created.Body.String())
	}
	updated := request(t, handler, http.MethodPut, "/servers/3", `{"name":"host-new","host":"192.0.2.4","username":"root"}`)
	if updated.Code != http.StatusOK || service.updated.Password != nil || strings.Contains(updated.Body.String(), "password") {
		t.Fatalf("update status=%d input=%+v body=%s", updated.Code, service.updated, updated.Body.String())
	}
	inventory := request(t, handler, http.MethodGet, "/servers/3/inventory", "")
	if inventory.Code != http.StatusOK || !strings.Contains(inventory.Body.String(), `"hosts":[]`) {
		t.Fatalf("inventory status=%d body=%s", inventory.Code, inventory.Body.String())
	}
}

func TestStandaloneAssetsAndAPI(t *testing.T) {
	db, err := sql.Open("sqlite", "file:esxi-standalone-test?mode=memory&cache=shared")
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	repo := esxiweb.NewRepository(db)
	if err := repo.Initialize(context.Background()); err != nil {
		t.Fatal(err)
	}
	handler := esxiweb.New(esxiweb.NewService(repo)).StandaloneHandler()
	for _, path := range []string{"/", "/v1/esxi/manifest.json", "/v1/esxi/assets/module.js", "/v1/esxi/assets/index.js"} {
		response := httptest.NewRecorder()
		handler.ServeHTTP(response, httptest.NewRequest(http.MethodGet, path, nil))
		if response.Code != http.StatusOK || response.Body.Len() == 0 {
			t.Fatalf("GET %s: status=%d body=%q", path, response.Code, response.Body.String())
		}
	}
}

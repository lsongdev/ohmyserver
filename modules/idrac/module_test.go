package idrac_test

import (
	"context"
	"database/sql"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	idrac "github.com/lsongdev/ohmyserver/modules/idrac"
	idracweb "github.com/lsongdev/ohmyserver/modules/idrac/web"
	_ "modernc.org/sqlite"
)

type apiService struct {
	idrac.Service
	created idrac.Input
	updated idrac.Input
	power   string
}

func (s *apiService) ListIDRAC(context.Context) ([]idrac.DTO, error) {
	return []idrac.DTO{{ID: 9, Name: "idrac", Host: "192.0.2.9", Username: "admin", HasCredential: true}}, nil
}
func (s *apiService) CreateIDRAC(_ context.Context, in idrac.Input) (idrac.DTO, error) {
	s.created = in
	return idrac.DTO{ID: 9, Name: in.Name, Host: in.Host, Username: in.Username, HasCredential: true}, nil
}
func (*apiService) GetIDRAC(context.Context, int64) (idrac.DTO, error) {
	return idrac.DTO{ID: 9, Name: "idrac", Host: "192.0.2.9", Username: "admin", HasCredential: true}, nil
}
func (s *apiService) UpdateIDRAC(_ context.Context, _ int64, in idrac.Input) (idrac.DTO, error) {
	s.updated = in
	return idrac.DTO{ID: 9, Name: in.Name, Host: in.Host, Username: in.Username, HasCredential: true}, nil
}
func (*apiService) DeleteIDRAC(context.Context, int64) error { return nil }
func (*apiService) TestIDRAC(context.Context, int64) error   { return nil }
func (*apiService) ReadIDRACInventory(context.Context, int64) (idrac.Inventory, error) {
	return idrac.Inventory{Sensors: []idrac.Sensor{}}, nil
}
func (s *apiService) ControlIDRACPower(_ context.Context, _ int64, action string) error {
	s.power = action
	return nil
}

func serve(t *testing.T, handler http.Handler, method, path, body string) *httptest.ResponseRecorder {
	t.Helper()
	request := httptest.NewRequest(method, path, strings.NewReader(body))
	if body != "" {
		request.Header.Set("Content-Type", "application/json")
	}
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	return response
}

func TestIDRACCredentialsAreWriteOnlyAndPowerIsValidated(t *testing.T) {
	service := &apiService{}
	handler := idracweb.New(service).Handler()
	created := serve(t, handler, http.MethodPost, "/servers", `{"name":"idrac","host":"192.0.2.9","username":"admin","password":"top-secret"}`)
	if created.Code != http.StatusCreated || service.created.Password == nil || *service.created.Password != "top-secret" {
		t.Fatalf("create status=%d request=%+v body=%s", created.Code, service.created, created.Body.String())
	}
	if strings.Contains(created.Body.String(), "top-secret") || strings.Contains(created.Body.String(), `"password"`) {
		t.Fatalf("create response leaked credential: %s", created.Body.String())
	}

	updated := serve(t, handler, http.MethodPut, "/servers/9", `{"name":"idrac-new","host":"192.0.2.10","username":"admin"}`)
	if updated.Code != http.StatusOK || service.updated.Password != nil {
		t.Fatalf("update status=%d password=%v body=%s", updated.Code, service.updated.Password, updated.Body.String())
	}
	if strings.Contains(updated.Body.String(), "password") {
		t.Fatalf("update response leaked password field: %s", updated.Body.String())
	}

	invalidPower := serve(t, handler, http.MethodPost, "/servers/9/power", `{"action":"erase-disks"}`)
	if invalidPower.Code != http.StatusUnprocessableEntity || service.power != "" {
		t.Fatalf("invalid power action status=%d action=%q body=%s", invalidPower.Code, service.power, invalidPower.Body.String())
	}
	validPower := serve(t, handler, http.MethodPost, "/servers/9/power", `{"action":"graceful"}`)
	if validPower.Code != http.StatusOK || service.power != "graceful" {
		t.Fatalf("power action status=%d action=%q body=%s", validPower.Code, service.power, validPower.Body.String())
	}
}

func TestStandaloneAssetsAndAPI(t *testing.T) {
	db, err := sql.Open("sqlite", "file:idrac-standalone-test?mode=memory&cache=shared")
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	repo := idracweb.NewRepository(db)
	if err := repo.Initialize(context.Background()); err != nil {
		t.Fatal(err)
	}
	handler := idracweb.New(idracweb.NewLocalService(repo)).StandaloneHandler()
	for _, path := range []string{"/", "/v1/idrac/manifest.json", "/v1/idrac/assets/module.js", "/v1/idrac/assets/index.js", "/v1/idrac/servers"} {
		response := httptest.NewRecorder()
		handler.ServeHTTP(response, httptest.NewRequest(http.MethodGet, path, nil))
		if response.Code != http.StatusOK || response.Body.Len() == 0 {
			t.Fatalf("GET %s: status=%d body=%q", path, response.Code, response.Body.String())
		}
	}
}

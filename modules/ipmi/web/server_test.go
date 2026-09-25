package web

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

type apiService struct {
	Service
	created CreateRequest
	updated UpdateRequest
	power   string
}

func (s *apiService) ListIPMIServers(context.Context) ([]ServerDTO, error) {
	return []ServerDTO{{ID: 7, Name: "bmc", BMCIP: "192.0.2.7", Username: "admin", HasCredential: true}}, nil
}
func (s *apiService) CreateIPMIServerAPI(_ context.Context, in CreateRequest) (*ServerDTO, error) {
	s.created = in
	return &ServerDTO{ID: 7, Name: in.Name, BMCIP: in.BMCIP, Port: in.Port, Username: in.Username, HasCredential: true}, nil
}
func (*apiService) ReadIPMIServer(context.Context, int64) (*ServerDTO, error) {
	return &ServerDTO{ID: 7, Name: "bmc", BMCIP: "192.0.2.7", Username: "admin", HasCredential: true}, nil
}
func (s *apiService) UpdateIPMIServerAPI(_ context.Context, _ int64, in UpdateRequest) (*ServerDTO, error) {
	s.updated = in
	return &ServerDTO{ID: 7, Name: in.Name, BMCIP: in.BMCIP, Username: in.Username, HasCredential: true}, nil
}
func (*apiService) RemoveIPMIServerAPI(context.Context, int64) error { return nil }
func (*apiService) TestIPMIConnection(context.Context, int64) error  { return nil }
func (*apiService) IPMIInventory(context.Context, int64) (*Inventory, error) {
	return &Inventory{FRUs: []FRU{}}, nil
}
func (s *apiService) ControlIPMIPower(_ context.Context, _ int64, action string) error {
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

func TestIPMICredentialsAreWriteOnlyAndPowerIsValidated(t *testing.T) {
	service := &apiService{}
	handler := New(service).Handler()
	created := serve(t, handler, http.MethodPost, "/servers", `{"name":"bmc","bmc_ip":"192.0.2.7","username":"admin","password":"top-secret"}`)
	if created.Code != http.StatusCreated || service.created.Password != "top-secret" || service.created.Port != 623 {
		t.Fatalf("create status=%d request=%+v body=%s", created.Code, service.created, created.Body.String())
	}
	if strings.Contains(created.Body.String(), "top-secret") || strings.Contains(created.Body.String(), `"password"`) {
		t.Fatalf("create response leaked credential: %s", created.Body.String())
	}

	updated := serve(t, handler, http.MethodPut, "/servers/7", `{"name":"bmc-new","bmc_ip":"192.0.2.8","username":"admin"}`)
	if updated.Code != http.StatusOK || service.updated.Password != nil {
		t.Fatalf("update status=%d password=%v body=%s", updated.Code, service.updated.Password, updated.Body.String())
	}
	if strings.Contains(updated.Body.String(), "password") {
		t.Fatalf("update response leaked password field: %s", updated.Body.String())
	}

	invalidPower := serve(t, handler, http.MethodPost, "/servers/7/power", `{"action":"erase-disks"}`)
	if invalidPower.Code != http.StatusUnprocessableEntity || service.power != "" {
		t.Fatalf("invalid power action status=%d action=%q body=%s", invalidPower.Code, service.power, invalidPower.Body.String())
	}
	validPower := serve(t, handler, http.MethodPost, "/servers/7/power", `{"action":"cycle"}`)
	if validPower.Code != http.StatusNoContent || service.power != "cycle" {
		t.Fatalf("power action status=%d action=%q body=%s", validPower.Code, service.power, validPower.Body.String())
	}
}

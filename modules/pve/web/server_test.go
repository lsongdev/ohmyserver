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
	created Input
	vm      CreateVMInput
	vmID    int64
}

func (*apiService) ListPVE(context.Context) ([]DTO, error) {
	return []DTO{{ID: 8, Name: "cluster", Host: "192.0.2.8", Username: "root@pam", HasCredential: true}}, nil
}
func (s *apiService) CreatePVE(_ context.Context, in Input) (DTO, error) {
	s.created = in
	return DTO{ID: 8, Name: in.Name, Host: in.Host, Username: in.Username, HasCredential: true}, nil
}
func (*apiService) GetPVE(context.Context, int64) (DTO, error) {
	return DTO{ID: 8, Name: "cluster", Host: "192.0.2.8", Username: "root@pam", HasCredential: true}, nil
}
func (*apiService) UpdatePVE(context.Context, int64, Input) (DTO, error) { return DTO{}, nil }
func (*apiService) DeletePVE(context.Context, int64) error               { return nil }
func (*apiService) TestPVE(context.Context, int64) error                 { return nil }
func (*apiService) PVEInventory(context.Context, int64) (Inventory, error) {
	return Inventory{Nodes: []Node{}}, nil
}
func (s *apiService) CreatePVEVM(_ context.Context, id int64, in CreateVMInput) (CreateVMResult, error) {
	s.vmID, s.vm = id, in
	return CreateVMResult{VMID: 101}, nil
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

func TestPVECredentialsAndVMCreationAPI(t *testing.T) {
	service := &apiService{}
	handler := New(service).Handler()
	created := request(t, handler, http.MethodPost, "/servers", `{"name":"cluster","host":"192.0.2.8","username":"root@pam","password":"secret"}`)
	if created.Code != http.StatusCreated || service.created.Password == nil || *service.created.Password != "secret" {
		t.Fatalf("create status=%d input=%+v body=%s", created.Code, service.created, created.Body.String())
	}
	if strings.Contains(created.Body.String(), "secret") || strings.Contains(created.Body.String(), `"password"`) {
		t.Fatalf("response leaked credential: %s", created.Body.String())
	}
	invalid := request(t, handler, http.MethodPost, "/servers/8/vms", `{"node":"pve1","template_id":0,"name":"new-vm"}`)
	if invalid.Code != http.StatusUnprocessableEntity || service.vmID != 0 {
		t.Fatalf("invalid VM request status=%d called=%d body=%s", invalid.Code, service.vmID, invalid.Body.String())
	}
	valid := request(t, handler, http.MethodPost, "/servers/8/vms", `{"node":"pve1","template_id":9000,"name":"new-vm","cores":4,"memory_mb":4096,"start":true}`)
	if valid.Code != http.StatusCreated || service.vmID != 8 || service.vm.TemplateID != 9000 || !strings.Contains(valid.Body.String(), `"vmid":101`) {
		t.Fatalf("VM create status=%d service=%+v body=%s", valid.Code, service, valid.Body.String())
	}
}

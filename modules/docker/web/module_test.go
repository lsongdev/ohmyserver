package web

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

type apiTestService struct {
	Service
	containerArgs [3]string
	containerID   string
	containerErr  error
	containerOps  []string
	composeOp     [2]string
	composeErr    error
	appOp         [2]string
	appErr        error
	pullName      string
	pullErr       error
}

func (s *apiTestService) CreateDockerContainer(_ context.Context, name, image, command string) (string, error) {
	s.containerArgs = [3]string{name, image, command}
	return "created-id", s.containerErr
}
func (s *apiTestService) StartDockerContainer(context.Context, string) error {
	s.containerOps = append(s.containerOps, "start")
	return s.containerErr
}
func (s *apiTestService) StopDockerContainer(context.Context, string) error {
	s.containerOps = append(s.containerOps, "stop")
	return s.containerErr
}
func (s *apiTestService) RestartDockerContainer(context.Context, string) error {
	s.containerOps = append(s.containerOps, "restart")
	return s.containerErr
}
func (s *apiTestService) DockerComposeAction(_ context.Context, name, action string) error {
	s.composeOp = [2]string{name, action}
	return s.composeErr
}
func (s *apiTestService) DockerAppAction(_ context.Context, id, action string) error {
	s.appOp = [2]string{id, action}
	return s.appErr
}
func (s *apiTestService) PullDockerImage(_ context.Context, name string, progress func(string) error) error {
	s.pullName = name
	if err := progress("downloading layer"); err != nil {
		return err
	}
	return s.pullErr
}

func TestCreateContainerValidatesAndNormalizesInput(t *testing.T) {
	service := &apiTestService{}
	handler := New(service).Handler()
	request := func(body string) *httptest.ResponseRecorder {
		t.Helper()
		req := httptest.NewRequest(http.MethodPost, "/containers", strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		response := httptest.NewRecorder()
		handler.ServeHTTP(response, req)
		return response
	}

	if response := request(`{"name":"  ","image":"alpine"}`); response.Code != http.StatusUnprocessableEntity {
		t.Fatalf("missing name: status=%d body=%s", response.Code, response.Body.String())
	}
	if response := request(`{"name":"app","image":"alpine","unexpected":true}`); response.Code != http.StatusBadRequest {
		t.Fatalf("unknown field: status=%d body=%s", response.Code, response.Body.String())
	}
	if service.containerArgs != [3]string{} {
		t.Fatalf("invalid requests reached Docker service: %#v", service.containerArgs)
	}
	response := request(`{"name":" app ","image":" alpine ","command":"serve"}`)
	if response.Code != http.StatusCreated || !strings.Contains(response.Body.String(), `"id":"created-id"`) {
		t.Fatalf("create container: status=%d body=%s", response.Code, response.Body.String())
	}
	if service.containerArgs != [3]string{"app", "alpine", "serve"} {
		t.Fatalf("service args = %#v", service.containerArgs)
	}
}

func TestDockerActionsAreAllowlistedAndDelegated(t *testing.T) {
	service := &apiTestService{}
	handler := New(service).Handler()
	request := func(method, path string) *httptest.ResponseRecorder {
		t.Helper()
		response := httptest.NewRecorder()
		handler.ServeHTTP(response, httptest.NewRequest(method, path, nil))
		return response
	}

	for _, action := range []string{"start", "stop", "restart"} {
		response := request(http.MethodPost, "/containers/c-1/"+action)
		if response.Code != http.StatusNoContent {
			t.Errorf("container %s: status=%d body=%s", action, response.Code, response.Body.String())
		}
	}
	if got := strings.Join(service.containerOps, ","); got != "start,stop,restart" {
		t.Fatalf("container actions = %q", got)
	}
	if response := request(http.MethodPost, "/compose/project-1/invalid"); response.Code != http.StatusBadRequest {
		t.Fatalf("unknown compose action: status=%d body=%s", response.Code, response.Body.String())
	}
	if service.composeOp != [2]string{} {
		t.Fatalf("invalid compose action reached service: %#v", service.composeOp)
	}
	if response := request(http.MethodPost, "/compose/project-1/restart"); response.Code != http.StatusNoContent || service.composeOp != [2]string{"project-1", "restart"} {
		t.Fatalf("compose action: status=%d call=%#v body=%s", response.Code, service.composeOp, response.Body.String())
	}
	if response := request(http.MethodPost, "/apps/app-1/invalid"); response.Code != http.StatusBadRequest {
		t.Fatalf("unknown app action: status=%d body=%s", response.Code, response.Body.String())
	}
	if service.appOp != [2]string{} {
		t.Fatalf("invalid app action reached service: %#v", service.appOp)
	}
	if response := request(http.MethodPost, "/apps/app-1/uninstall"); response.Code != http.StatusNoContent || service.appOp != [2]string{"app-1", "uninstall"} {
		t.Fatalf("app action: status=%d call=%#v body=%s", response.Code, service.appOp, response.Body.String())
	}
}

func TestPullImageStreamsProgressAndTerminalEvent(t *testing.T) {
	for _, test := range []struct {
		name    string
		service *apiTestService
		want    string
	}{
		{name: "done", service: &apiTestService{}, want: "event: done"},
		{name: "error", service: &apiTestService{pullErr: errors.New("registry unavailable")}, want: "event: error"},
	} {
		t.Run(test.name, func(t *testing.T) {
			handler := New(test.service).Handler()
			req := httptest.NewRequest(http.MethodPost, "/images/pull", strings.NewReader(`{"name":" alpine "}`))
			req.Header.Set("Content-Type", "application/json")
			response := httptest.NewRecorder()
			handler.ServeHTTP(response, req)
			body := response.Body.String()
			if response.Code != http.StatusOK || response.Header().Get("Content-Type") != "text/event-stream; charset=utf-8" || !strings.Contains(body, "event: progress") || !strings.Contains(body, test.want) {
				t.Fatalf("status=%d headers=%v body=%s", response.Code, response.Header(), body)
			}
			if test.service.pullName != "alpine" {
				t.Fatalf("pull name = %q", test.service.pullName)
			}
		})
	}
}

func TestStandaloneHandlerServesPageManifestAssetsAndAPI(t *testing.T) {
	handler := New(&apiTestService{}).StandaloneHandler()
	for _, test := range []struct{ path, contains string }{
		{"/", `name="api-base"`},
		{"/v1/docker/manifest.json", `assets/module.js`},
		{"/v1/docker/assets/module.js", `createAPI`},
		{"/v1/docker/assets/index.js", `lsong.org/scripts/react/index.js`},
	} {
		response := httptest.NewRecorder()
		handler.ServeHTTP(response, httptest.NewRequest(http.MethodGet, test.path, nil))
		if response.Code != http.StatusOK || !strings.Contains(response.Body.String(), test.contains) {
			t.Fatalf("GET %s status=%d body=%q", test.path, response.Code, response.Body.String())
		}
	}
}

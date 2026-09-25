package yeelight_test

import (
	"context"
	"database/sql"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	yeelight "github.com/lsongdev/ohmyserver/modules/yeelight"
	yeelightweb "github.com/lsongdev/ohmyserver/modules/yeelight/web"
	_ "modernc.org/sqlite"
)

type apiService struct {
	yeelight.Service
	created       yeelight.Input
	updated       yeelight.Input
	controlAction string
	controlColor  string
	controlCalls  int
}

func (*apiService) ListYeelight(context.Context) ([]yeelight.Device, error) {
	return []yeelight.Device{}, nil
}
func (s *apiService) CreateYeelight(_ context.Context, input yeelight.Input) (yeelight.Device, error) {
	s.created = input
	return yeelight.Device{ID: 1, Name: input.Name, Host: input.Host, Port: input.Port}, nil
}
func (*apiService) GetYeelight(context.Context, int64) (yeelight.Device, error) {
	return yeelight.Device{ID: 1}, nil
}
func (s *apiService) UpdateYeelight(_ context.Context, _ int64, input yeelight.Input) (yeelight.Device, error) {
	s.updated = input
	return yeelight.Device{ID: 1, Name: input.Name, Host: input.Host, Port: input.Port}, nil
}
func (*apiService) DeleteYeelight(context.Context, int64) error { return nil }
func (*apiService) YeelightState(context.Context, int64) (yeelight.State, error) {
	return yeelight.State{}, nil
}
func (*apiService) TestYeelight(context.Context, int64) error { return nil }
func (s *apiService) ControlYeelight(_ context.Context, _ int64, action string, _ int, color string) (yeelight.State, error) {
	s.controlAction, s.controlColor = action, color
	s.controlCalls++
	return yeelight.State{Power: "on"}, nil
}
func (*apiService) DiscoverYeelight(context.Context) ([]yeelight.FoundDevice, error) {
	return []yeelight.FoundDevice{}, nil
}

func TestYeelightDeviceInputDefaultsAndValidation(t *testing.T) {
	service := &apiService{}
	handler := yeelightweb.New(service).Handler()
	call := func(method, path, body string) *httptest.ResponseRecorder {
		t.Helper()
		w := httptest.NewRecorder()
		handler.ServeHTTP(w, httptest.NewRequest(method, path, strings.NewReader(body)))
		return w
	}
	created := call(http.MethodPost, "/devices", `{"host":"  192.0.2.20  "}`)
	if created.Code != http.StatusCreated || service.created.Name != "192.0.2.20" || service.created.Host != "192.0.2.20" || service.created.Port != 55443 {
		t.Fatalf("create defaults: status=%d body=%s input=%+v", created.Code, created.Body.String(), service.created)
	}
	if response := call(http.MethodPost, "/devices", `{"host":"host","port":70000}`); response.Code != http.StatusUnprocessableEntity {
		t.Fatalf("invalid port: status=%d body=%s", response.Code, response.Body.String())
	}
	if response := call(http.MethodPut, "/devices/1", `{"name":" bulb ","host":" host ","port":0}`); response.Code != http.StatusOK || service.updated.Name != "bulb" || service.updated.Host != "host" || service.updated.Port != 55443 {
		t.Fatalf("update defaults: status=%d body=%s input=%+v", response.Code, response.Body.String(), service.updated)
	}
}

func TestYeelightControlRejectsInvalidRGBBeforeDeviceCall(t *testing.T) {
	service := &apiService{}
	handler := yeelightweb.New(service).Handler()
	for _, color := range []string{"#12", "#zzzzzz"} {
		response := httptest.NewRecorder()
		handler.ServeHTTP(response, httptest.NewRequest(http.MethodPost, "/devices/1/control", strings.NewReader(`{"action":"color","color":"`+color+`"}`)))
		if response.Code != http.StatusUnprocessableEntity {
			t.Errorf("invalid color %q: status=%d body=%s", color, response.Code, response.Body.String())
		}
	}
	if service.controlCalls != 0 {
		t.Fatalf("invalid colors reached device service %d times", service.controlCalls)
	}
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, httptest.NewRequest(http.MethodPost, "/devices/1/control", strings.NewReader(`{"action":"color","color":"#A0b1C2"}`)))
	if response.Code != http.StatusOK || service.controlCalls != 1 || service.controlColor != "#A0b1C2" {
		t.Fatalf("valid color: status=%d body=%s calls=%d color=%q", response.Code, response.Body.String(), service.controlCalls, service.controlColor)
	}
}

func TestStandaloneAssetsAndAPI(t *testing.T) {
	db, err := sql.Open("sqlite", "file:yeelight-standalone-test?mode=memory&cache=shared")
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	repo := yeelightweb.NewRepository(db)
	if err := repo.Initialize(context.Background()); err != nil {
		t.Fatal(err)
	}
	handler := yeelightweb.New(yeelightweb.NewLocalService(repo)).StandaloneHandler()
	for _, path := range []string{"/", "/v1/yeelight/manifest.json", "/v1/yeelight/assets/module.js", "/v1/yeelight/assets/index.js", "/v1/yeelight/devices"} {
		response := httptest.NewRecorder()
		handler.ServeHTTP(response, httptest.NewRequest(http.MethodGet, path, nil))
		if response.Code != http.StatusOK || response.Body.Len() == 0 {
			t.Fatalf("GET %s: status=%d body=%q", path, response.Code, response.Body.String())
		}
	}
}

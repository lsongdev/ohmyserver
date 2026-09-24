package web

import (
	"context"
	"net/http/httptest"
	"strings"
	"testing"

	dashboard "github.com/lsongdev/ohmyserver/modules/dashboard"
)

type testService struct{}

func (testService) Overview(context.Context) (dashboard.Overview, error) {
	return dashboard.Overview{Widgets: []dashboard.Widget{{ID: 2, Name: "CPU", Type: "system_info"}}, System: dashboard.SystemSummary{Hostname: "test-host", CPUUsage: 12.5}}, nil
}

func TestGetDashboardOverview(t *testing.T) {
	response := httptest.NewRecorder()
	New(testService{}).Handler().ServeHTTP(response, httptest.NewRequest("GET", "/overview", nil))
	if response.Code != 200 || !strings.Contains(response.Body.String(), `"name":"CPU"`) || !strings.Contains(response.Body.String(), `"hostname":"test-host"`) {
		t.Fatalf("status=%d body=%s", response.Code, response.Body.String())
	}
}

func TestHandlerServesModuleManifestAndEntry(t *testing.T) {
	handler := New(testService{}).Handler()
	for _, path := range []string{"/manifest.json", "/assets/module.js", "/assets/index.js"} {
		response := httptest.NewRecorder()
		handler.ServeHTTP(response, httptest.NewRequest("GET", path, nil))
		if response.Code != 200 || response.Body.Len() == 0 {
			t.Fatalf("GET %s: status=%d body=%q", path, response.Code, response.Body.String())
		}
	}
}

func TestStandaloneHandlerServesPageAssetsAndOverviewAPI(t *testing.T) {
	handler := New(testService{}).StandaloneHandler()
	for path, contains := range map[string]string{
		"/":                             "api-base",
		"/v1/dashboard/manifest.json":   "assets/module.js",
		"/v1/dashboard/assets/index.js": "lsong.org/scripts/react/index.js",
		"/v1/dashboard/overview":        `"hostname":"test-host"`,
	} {
		response := httptest.NewRecorder()
		handler.ServeHTTP(response, httptest.NewRequest("GET", path, nil))
		if response.Code != 200 || !strings.Contains(response.Body.String(), contains) {
			t.Errorf("GET %s: status=%d body=%s", path, response.Code, response.Body.String())
		}
	}
}

func TestLocalServiceComposesInjectedSources(t *testing.T) {
	service := NewLocalService(testWidgets{}, testSystem{})
	result, err := service.Overview(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Widgets) != 1 || result.System.Hostname != "standalone-test" {
		t.Fatalf("overview = %#v", result)
	}
}

type testWidgets struct{}

func (testWidgets) EnabledWidgets(context.Context) ([]dashboard.Widget, error) {
	return []dashboard.Widget{{ID: 1, Name: "CPU", Type: "system_info"}}, nil
}

type testSystem struct{}

func (testSystem) SystemSummary(context.Context) (dashboard.SystemSummary, error) {
	return dashboard.SystemSummary{Hostname: "standalone-test"}, nil
}

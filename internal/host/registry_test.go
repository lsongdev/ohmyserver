package host

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"reflect"
	"sort"
	"strings"
	"testing"
)

func TestFrontendAndBackendModuleRegistriesMatch(t *testing.T) {
	registry, err := NewAPIModules(&Server{})
	if err != nil {
		t.Fatal(err)
	}
	var frontendIDs []string
	var descriptors struct {
		Modules []struct {
			ID string `json:"id"`
		} `json:"modules"`
	}
	response := httptest.NewRecorder()
	registry.IndexHandler("").ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/v1/modules", nil))
	if err := json.Unmarshal(response.Body.Bytes(), &descriptors); err != nil {
		t.Fatal(err)
	}
	for _, module := range descriptors.Modules {
		frontendIDs = append(frontendIDs, module.ID)
	}
	backendIDs := registry.Names()
	sort.Strings(frontendIDs)
	if !reflect.DeepEqual(frontendIDs, backendIDs) {
		t.Fatalf("frontend modules %v do not match backend modules %v", frontendIDs, backendIDs)
	}
}

func TestAPIModuleRegistryMountsFeaturePrefixes(t *testing.T) {
	registry, err := NewAPIModules(&Server{})
	if err != nil {
		t.Fatal(err)
	}
	handler := http.StripPrefix("/v1", registry.Handler())
	probes := map[string]string{
		"acme": "/certificates", "clash": "/servers", "cron": "/jobs", "dashboard": "/overview",
		"dhcp": "/settings", "disks": "/usage", "dns": "/settings", "docker": "/containers",
		"esxi": "/servers", "files": "/entries", "forward": "/listeners", "idrac": "/servers",
		"ipmi": "/servers", "monitor": "/hosts", "ntp": "/settings", "openwrt": "/routers",
		"packages": "/packages", "projects": "/projects", "pve": "/servers", "settings": "/",
		"speedtest": "/results", "ssh": "/servers/test", "system": "/info", "terminal": "/local/ws",
		"uptime": "/monitors", "webserver": "/listeners", "widgets": "/widgets", "yeelight": "/devices",
	}
	for module, path := range probes {
		t.Run(module, func(t *testing.T) {
			request := httptest.NewRequest(http.MethodOptions, "/v1/"+module+path, nil)
			response := httptest.NewRecorder()
			handler.ServeHTTP(response, request)
			if response.Code != http.StatusMethodNotAllowed || response.Header().Get("Content-Type") != "application/json; charset=utf-8" || !strings.Contains(response.Body.String(), "method_not_allowed") {
				t.Fatalf("module route not mounted with normalized API behavior: status=%d content-type=%q body=%s", response.Code, response.Header().Get("Content-Type"), response.Body.String())
			}
		})
	}
}

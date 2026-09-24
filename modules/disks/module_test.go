package disks_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	disks "github.com/lsongdev/ohmyserver/modules/disks"
	disksweb "github.com/lsongdev/ohmyserver/modules/disks/web"
)

type fakeService struct{}

func (fakeService) DiskUsage(context.Context) ([]disks.Usage, error) {
	return []disks.Usage{{Device: "/dev/vda1", MountPoint: "/", TotalGB: 100, UsedGB: 25, FreeGB: 75, UsePercent: 25}}, nil
}
func (fakeService) DiskDevices(context.Context) ([]string, error) { return []string{"/dev/vda"}, nil }
func (fakeService) DiskHealth(_ context.Context, device string) (disks.Health, error) {
	return disks.Health{Device: device, Healthy: true, Temperature: "35°C"}, nil
}

func TestModuleDiskUsageAndHealth(t *testing.T) {
	mux := disksweb.New(fakeService{}).Handler()
	for _, test := range []struct{ path, want string }{{"/usage", "/dev/vda1"}, {"/devices", "/dev/vda"}, {"/health?device=%2Fdev%2Fvda", "35°C"}} {
		response := httptest.NewRecorder()
		mux.ServeHTTP(response, httptest.NewRequest(http.MethodGet, test.path, nil))
		if response.Code != http.StatusOK || !strings.Contains(response.Body.String(), test.want) {
			t.Fatalf("GET %s: status=%d body=%s", test.path, response.Code, response.Body.String())
		}
	}
}

func TestModuleRequiresDeviceForHealth(t *testing.T) {
	response := httptest.NewRecorder()
	disksweb.New(fakeService{}).Handler().ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/health", nil))
	if response.Code != http.StatusBadRequest || !strings.Contains(response.Body.String(), "invalid_device") {
		t.Fatalf("status=%d body=%s", response.Code, response.Body.String())
	}
}

func TestStandaloneAssetAndAPIPaths(t *testing.T) {
	handler := disksweb.New(fakeService{}).StandaloneHandler()
	for _, path := range []string{"/", "/v1/disks/manifest.json", "/v1/disks/assets/module.js", "/v1/disks/assets/index.js"} {
		response := httptest.NewRecorder()
		handler.ServeHTTP(response, httptest.NewRequest(http.MethodGet, path, nil))
		if response.Code != http.StatusOK || response.Body.Len() == 0 {
			t.Fatalf("GET %s: status=%d body=%q", path, response.Code, response.Body.String())
		}
	}
}

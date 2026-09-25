package service

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"

	packagesmodule "github.com/lsongdev/ohmyserver/modules/packages"
	"github.com/lsongdev/ohmyserver/modules/packages/pkgmgr"
	packagesweb "github.com/lsongdev/ohmyserver/modules/packages/web"
)

type fakePackageManager struct {
	installed, upgradable, search []pkgmgr.Package
	operation                     string
	packages                      []string
}

func (m *fakePackageManager) Backend() pkgmgr.Backend { return pkgmgr.APT }
func (m *fakePackageManager) Name() string            { return "APT" }
func (m *fakePackageManager) Executable() string      { return "/usr/bin/apt-get" }
func (m *fakePackageManager) Capabilities() pkgmgr.Capabilities {
	return pkgmgr.Capabilities{Search: true, Info: true, ListInstalled: true, ListUpgradable: true, Refresh: true, Install: true, Remove: true, Upgrade: true, SafeUpgradeAll: true}
}
func (m *fakePackageManager) ListInstalled(context.Context) ([]pkgmgr.Package, error) {
	return m.installed, nil
}
func (m *fakePackageManager) ListUpgradable(context.Context) ([]pkgmgr.Package, error) {
	return m.upgradable, nil
}
func (m *fakePackageManager) Search(context.Context, string) ([]pkgmgr.Package, error) {
	return m.search, nil
}
func (m *fakePackageManager) Refresh(context.Context, pkgmgr.ChangeOptions) (pkgmgr.Result, error) {
	m.operation = "refresh"
	return pkgmgr.Result{Stdout: "refreshed"}, nil
}
func (m *fakePackageManager) Install(_ context.Context, items []string, _ pkgmgr.ChangeOptions) (pkgmgr.Result, error) {
	m.operation = "install"
	m.packages = append([]string(nil), items...)
	return pkgmgr.Result{}, nil
}
func (m *fakePackageManager) Remove(_ context.Context, items []string, _ pkgmgr.ChangeOptions) (pkgmgr.Result, error) {
	m.operation = "remove"
	m.packages = append([]string(nil), items...)
	return pkgmgr.Result{}, nil
}
func (m *fakePackageManager) Upgrade(_ context.Context, items []string, _ pkgmgr.ChangeOptions) (pkgmgr.Result, error) {
	m.operation = "upgrade"
	m.packages = append([]string(nil), items...)
	return pkgmgr.Result{}, nil
}

func packageTestSystem(manager PackageManager) *SystemService {
	return NewSystemServiceWithManagerFactory(func() (PackageManager, error) { return manager, nil })
}

func TestPackagesModuleListsAndSortsPackages(t *testing.T) {
	manager := &fakePackageManager{installed: []pkgmgr.Package{{Name: "zlib", Version: "1.3", Installed: true}, {Name: "curl", Version: "8.1", Installed: true}}}
	module := packagesweb.New(packageTestSystem(manager))
	request := httptest.NewRequest(http.MethodGet, "/packages?view=installed", nil)
	response := httptest.NewRecorder()
	module.Handler().ServeHTTP(response, request)
	if response.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", response.Code, response.Body.String())
	}
	var payload struct {
		Data packagesmodule.Catalog `json:"data"`
	}
	if err := json.NewDecoder(response.Body).Decode(&payload); err != nil {
		t.Fatal(err)
	}
	if payload.Data.Backend != "apt" || payload.Data.Manager != "APT" || len(payload.Data.Packages) != 2 || payload.Data.Packages[0].Name != "curl" {
		t.Fatalf("catalog=%#v", payload.Data)
	}
}

func TestPackagesModuleSearchAndMutation(t *testing.T) {
	manager := &fakePackageManager{search: []pkgmgr.Package{{Name: "ripgrep", Description: "fast search"}}}
	module := packagesweb.New(packageTestSystem(manager))
	response := httptest.NewRecorder()
	module.Handler().ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/packages?view=search&q=rg", nil))
	if response.Code != 200 || !strings.Contains(response.Body.String(), "fast search") {
		t.Fatalf("status=%d body=%s", response.Code, response.Body.String())
	}
	response = httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, "/remove", strings.NewReader(`{"name":"curl"}`))
	request.Header.Set("Content-Type", "application/json")
	module.Handler().ServeHTTP(response, request)
	if response.Code != 200 || manager.operation != "remove" || !reflect.DeepEqual(manager.packages, []string{"curl"}) {
		t.Fatalf("status=%d operation=%q packages=%#v body=%s", response.Code, manager.operation, manager.packages, response.Body.String())
	}
}

func TestPackagesModuleRejectsInvalidRequests(t *testing.T) {
	module := packagesweb.New(packageTestSystem(&fakePackageManager{}))
	response := httptest.NewRecorder()
	module.Handler().ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/packages?view=unknown", nil))
	if response.Code != http.StatusBadRequest || !strings.Contains(response.Body.String(), "unknown package view") {
		t.Fatalf("status=%d body=%s", response.Code, response.Body.String())
	}
}

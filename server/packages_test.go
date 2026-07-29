package server

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"net/url"
	"reflect"
	"strings"
	"testing"

	"github.com/lsongdev/ohmyserver/pkgmgr"
)

type fakePackageManager struct {
	installed  []pkgmgr.Package
	upgradable []pkgmgr.Package
	search     []pkgmgr.Package
	operation  string
	packages   []string
}

func (m *fakePackageManager) Backend() pkgmgr.Backend { return pkgmgr.APT }
func (m *fakePackageManager) Name() string            { return "APT" }
func (m *fakePackageManager) Executable() string      { return "/usr/bin/apt-get" }
func (m *fakePackageManager) Capabilities() pkgmgr.Capabilities {
	return pkgmgr.Capabilities{
		Search: true, Info: true, ListInstalled: true, ListUpgradable: true,
		Refresh: true, Install: true, Remove: true, Upgrade: true,
		SafeUpgradeAll: true,
	}
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
	return pkgmgr.Result{}, nil
}
func (m *fakePackageManager) Install(_ context.Context, packages []string, _ pkgmgr.ChangeOptions) (pkgmgr.Result, error) {
	m.operation, m.packages = "install", append([]string(nil), packages...)
	return pkgmgr.Result{}, nil
}
func (m *fakePackageManager) Remove(_ context.Context, packages []string, _ pkgmgr.ChangeOptions) (pkgmgr.Result, error) {
	m.operation, m.packages = "remove", append([]string(nil), packages...)
	return pkgmgr.Result{}, nil
}
func (m *fakePackageManager) Upgrade(_ context.Context, packages []string, _ pkgmgr.ChangeOptions) (pkgmgr.Result, error) {
	m.operation, m.packages = "upgrade", append([]string(nil), packages...)
	return pkgmgr.Result{}, nil
}

func packageTestServer(manager packageManagerRuntime) *Server {
	return &Server{
		packageManagerFactory: func() (packageManagerRuntime, error) {
			return manager, nil
		},
	}
}

func TestSystemPackagesView(t *testing.T) {
	manager := &fakePackageManager{
		installed: []pkgmgr.Package{
			{Name: "zlib", Version: "1.3", Installed: true},
			{Name: "curl", Version: "8.1", Architecture: "amd64", Installed: true},
		},
	}
	request := httptest.NewRequest(http.MethodGet, "/packages", nil)
	response := httptest.NewRecorder()

	packageTestServer(manager).SystemPackagesView(response, request)

	if response.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", response.Code, response.Body.String())
	}
	body := response.Body.String()
	for _, expected := range []string{
		"System Packages", "APT", "/usr/bin/apt-get", "curl", "zlib",
		`class="input"`, `class="button`,
	} {
		if !strings.Contains(body, expected) {
			t.Fatalf("response does not contain %q", expected)
		}
	}
	if strings.Index(body, "curl") > strings.Index(body, "zlib") {
		t.Fatal("packages were not sorted by name")
	}
}

func TestSystemPackagesSearchView(t *testing.T) {
	manager := &fakePackageManager{
		search: []pkgmgr.Package{{Name: "ripgrep", Description: "fast search"}},
	}
	request := httptest.NewRequest(http.MethodGet, "/packages?view=search&q=ripgrep", nil)
	response := httptest.NewRecorder()

	packageTestServer(manager).SystemPackagesView(response, request)

	if response.Code != http.StatusOK || !strings.Contains(response.Body.String(), "fast search") {
		t.Fatalf("status = %d, body = %s", response.Code, response.Body.String())
	}
}

func TestSystemPackagesUnavailable(t *testing.T) {
	server := &Server{
		packageManagerFactory: func() (packageManagerRuntime, error) {
			return nil, pkgmgr.ErrNotFound
		},
	}
	request := httptest.NewRequest(http.MethodGet, "/packages", nil)
	response := httptest.NewRecorder()

	server.SystemPackagesView(response, request)

	if response.Code != http.StatusOK || !strings.Contains(response.Body.String(), "No supported package manager") {
		t.Fatalf("status = %d, body = %s", response.Code, response.Body.String())
	}
}

func TestSystemPackageMutations(t *testing.T) {
	tests := []struct {
		name          string
		method        string
		target        string
		form          url.Values
		handler       func(*Server, http.ResponseWriter, *http.Request)
		wantOperation string
		wantPackages  []string
	}{
		{
			name: "refresh", method: http.MethodPost, target: "/packages/refresh",
			handler: (*Server).SystemPackagesRefreshHandler, wantOperation: "refresh",
		},
		{
			name: "install", method: http.MethodPost, target: "/packages/install",
			form:          url.Values{"package": {"curl"}},
			handler:       (*Server).SystemPackagesInstallHandler,
			wantOperation: "install", wantPackages: []string{"curl"},
		},
		{
			name: "remove", method: http.MethodDelete, target: "/packages/remove?name=curl",
			handler:       (*Server).SystemPackagesRemoveHandler,
			wantOperation: "remove", wantPackages: []string{"curl"},
		},
		{
			name: "upgrade one", method: http.MethodPost, target: "/packages/upgrade?name=curl",
			handler:       (*Server).SystemPackagesUpgradeHandler,
			wantOperation: "upgrade", wantPackages: []string{"curl"},
		},
		{
			name: "upgrade all", method: http.MethodPost, target: "/packages/upgrade",
			handler: (*Server).SystemPackagesUpgradeHandler, wantOperation: "upgrade",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			manager := &fakePackageManager{}
			server := packageTestServer(manager)
			var body *strings.Reader
			if test.form != nil {
				body = strings.NewReader(test.form.Encode())
			} else {
				body = strings.NewReader("")
			}
			request := httptest.NewRequest(test.method, test.target, body)
			if test.form != nil {
				request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
			}
			response := httptest.NewRecorder()

			test.handler(server, response, request)

			if response.Code != http.StatusSeeOther {
				t.Fatalf("status = %d, body = %s", response.Code, response.Body.String())
			}
			if manager.operation != test.wantOperation {
				t.Fatalf("operation = %q, want %q", manager.operation, test.wantOperation)
			}
			if !reflect.DeepEqual(manager.packages, test.wantPackages) {
				t.Fatalf("packages = %#v, want %#v", manager.packages, test.wantPackages)
			}
			if location := response.Header().Get("Location"); !strings.HasPrefix(location, "/packages?") {
				t.Fatalf("location = %q", location)
			}
		})
	}
}

func TestSystemPackageHandlersRejectWrongMethods(t *testing.T) {
	server := packageTestServer(&fakePackageManager{})
	tests := []struct {
		target  string
		handler http.HandlerFunc
	}{
		{"/packages/refresh", server.SystemPackagesRefreshHandler},
		{"/packages/install", server.SystemPackagesInstallHandler},
		{"/packages/remove?name=curl", server.SystemPackagesRemoveHandler},
		{"/packages/upgrade", server.SystemPackagesUpgradeHandler},
	}
	for _, test := range tests {
		response := httptest.NewRecorder()
		test.handler(response, httptest.NewRequest(http.MethodGet, test.target, nil))
		if response.Code != http.StatusMethodNotAllowed {
			t.Fatalf("%s status = %d", test.target, response.Code)
		}
	}
}

func TestSystemPackagesViewReportsQueryError(t *testing.T) {
	server := &Server{
		packageManagerFactory: func() (packageManagerRuntime, error) {
			return nil, errors.New("probe failed")
		},
	}
	response := httptest.NewRecorder()
	server.SystemPackagesView(response, httptest.NewRequest(http.MethodGet, "/packages", nil))
	if !strings.Contains(response.Body.String(), "probe failed") {
		t.Fatalf("body = %s", response.Body.String())
	}
}

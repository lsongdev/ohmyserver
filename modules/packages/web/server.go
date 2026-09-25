package web

import (
	"encoding/json"
	"errors"
	"net/http"
	"strings"

	packages "github.com/lsongdev/ohmyserver/modules/packages"
)

type Module struct{ service packages.Service }

func New(service packages.Service) *Module { return &Module{service: service} }
func (*Module) Name() string               { return "packages" }
func (m *Module) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /packages", func(w http.ResponseWriter, r *http.Request) {
		view := r.URL.Query().Get("view")
		if view == "" {
			view = "installed"
		}
		catalog, err := m.service.PackageCatalog(r.Context(), view, r.URL.Query().Get("q"))
		if err != nil {
			packageError(w, err)
			return
		}
		packageJSON(w, 200, map[string]any{"data": catalog})
	})
	mux.HandleFunc("POST /refresh", func(w http.ResponseWriter, r *http.Request) { m.change(w, r, "refresh", "") })
	mux.HandleFunc("POST /install", func(w http.ResponseWriter, r *http.Request) {
		var body struct {
			Name string `json:"name"`
		}
		if json.NewDecoder(r.Body).Decode(&body) != nil || strings.TrimSpace(body.Name) == "" {
			packageProblem(w, 400, "invalid_package", "package name is required")
			return
		}
		m.change(w, r, "install", body.Name)
	})
	mux.HandleFunc("POST /remove", func(w http.ResponseWriter, r *http.Request) {
		var body struct {
			Name string `json:"name"`
		}
		if json.NewDecoder(r.Body).Decode(&body) != nil || strings.TrimSpace(body.Name) == "" {
			packageProblem(w, 400, "invalid_package", "package name is required")
			return
		}
		m.change(w, r, "remove", body.Name)
	})
	mux.HandleFunc("POST /upgrade", func(w http.ResponseWriter, r *http.Request) {
		var body struct {
			Name string `json:"name"`
		}
		if r.Body != nil && r.ContentLength != 0 {
			if json.NewDecoder(r.Body).Decode(&body) != nil {
				packageProblem(w, 400, "invalid_body", "invalid JSON request")
				return
			}
		}
		m.change(w, r, "upgrade", body.Name)
	})
	return wrapModuleWeb(mux)
}
func (m *Module) change(w http.ResponseWriter, r *http.Request, operation, name string) {
	result, err := m.service.ChangePackages(r.Context(), operation, strings.TrimSpace(name))
	if err != nil {
		packageError(w, err)
		return
	}
	packageJSON(w, http.StatusOK, map[string]any{"data": result})
}
func packageJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}
func packageProblem(w http.ResponseWriter, status int, code, message string) {
	packageJSON(w, status, map[string]any{"error": map[string]string{"code": code, "message": message}})
}
func packageError(w http.ResponseWriter, err error) {
	status := http.StatusInternalServerError
	if errors.Is(err, packages.ErrInvalidRequest) {
		status = http.StatusBadRequest
	} else if errors.Is(err, packages.ErrUnavailable) {
		status = http.StatusServiceUnavailable
	}
	packageProblem(w, status, "package_error", err.Error())
}

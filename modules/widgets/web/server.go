package web

import (
	"encoding/json"
	"net/http"
	"strconv"
	"strings"

	widgets "github.com/lsongdev/ohmyserver/modules/widgets"
)

type Widget = widgets.Widget

type Module struct{ repo widgets.Repository }

func New(repo widgets.Repository) *Module { return &Module{repo: repo} }
func (*Module) Name() string              { return "widgets" }
func (m *Module) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /widgets", func(w http.ResponseWriter, r *http.Request) {
		items, err := m.repo.List(r.Context(), false)
		if err != nil {
			problem(w, err)
			return
		}
		respond(w, 200, map[string]any{"data": items})
	})
	mux.HandleFunc("POST /widgets", func(w http.ResponseWriter, r *http.Request) {
		var x Widget
		if json.NewDecoder(r.Body).Decode(&x) != nil || !valid(x) {
			fail(w, 400, "invalid_widget", "name and type are required")
			return
		}
		if err := m.repo.Create(r.Context(), &x); err != nil {
			problem(w, err)
			return
		}
		respond(w, 201, map[string]any{"data": x})
	})
	mux.HandleFunc("PUT /widgets/{id}", func(w http.ResponseWriter, r *http.Request) {
		id, err := idFrom(r)
		if err != nil {
			fail(w, 400, "invalid_id", "widget id must be positive")
			return
		}
		var x Widget
		if json.NewDecoder(r.Body).Decode(&x) != nil || !valid(x) {
			fail(w, 400, "invalid_widget", "name and type are required")
			return
		}
		x.ID = id
		if err := m.repo.Update(r.Context(), x); err != nil {
			problem(w, err)
			return
		}
		w.WriteHeader(204)
	})
	mux.HandleFunc("DELETE /widgets/{id}", func(w http.ResponseWriter, r *http.Request) {
		id, err := idFrom(r)
		if err != nil {
			fail(w, 400, "invalid_id", "widget id must be positive")
			return
		}
		if err := m.repo.Delete(r.Context(), id); err != nil {
			problem(w, err)
			return
		}
		w.WriteHeader(204)
	})
	return wrapModuleWeb(mux)
}

func (m *Module) StandaloneHandler() http.Handler {
	mux := http.NewServeMux()
	mux.Handle("/v1/widgets/", http.StripPrefix("/v1/widgets", m.Handler()))
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/" {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		http.ServeFileFS(w, r, moduleWeb, "assets/index.html")
	})
	return mux
}
func valid(x Widget) bool { return strings.TrimSpace(x.Name) != "" && strings.TrimSpace(x.Type) != "" }
func idFrom(r *http.Request) (int64, error) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil || id < 1 {
		return 0, http.ErrMissingFile
	}
	return id, nil
}
func respond(w http.ResponseWriter, status int, data any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(data)
}
func fail(w http.ResponseWriter, status int, code, message string) {
	respond(w, status, map[string]any{"error": map[string]string{"code": code, "message": message}})
}
func problem(w http.ResponseWriter, err error) { fail(w, 500, "widgets_error", err.Error()) }

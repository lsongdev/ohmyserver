// Package app composes independently owned feature HTTP modules.
package app

import (
	"encoding/json"
	"fmt"
	"net/http"
	"regexp"
	"sort"
)

var moduleName = regexp.MustCompile(`^[a-z][a-z0-9-]*$`)

type HTTPModule interface {
	Name() string
	Handler() http.Handler
}

type Modules struct{ items map[string]HTTPModule }

type PublicModule struct {
	ID       string `json:"id"`
	APIBase  string `json:"apiBase"`
	PageBase string `json:"pageBase"`
}

func NewModules() *Modules { return &Modules{items: make(map[string]HTTPModule)} }

// Names returns the registered module identifiers in stable order. The shell
// uses this for diagnostics and to verify its frontend/backend module manifest.
func (m *Modules) Names() []string {
	names := make([]string, 0, len(m.items))
	for name := range m.items {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

// PublicModules exposes only modules explicitly registered by the Go host.
func (m *Modules) PublicModules(basePath string) []PublicModule {
	items := make([]PublicModule, 0, len(m.items))
	for _, id := range m.Names() {
		pageBase := "/" + id
		if id == "dashboard" {
			pageBase = "/"
		}
		items = append(items, PublicModule{ID: id, APIBase: basePath + "/v1/" + id, PageBase: pageBase})
	}
	return items
}

func (m *Modules) IndexHandler(basePath string) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet && r.Method != http.MethodHead {
			w.Header().Set("Allow", "GET, HEAD")
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		w.Header().Set("Cache-Control", "no-cache")
		w.Header().Set("Content-Type", "application/json; charset=utf-8")
		_ = json.NewEncoder(w).Encode(map[string]any{"modules": m.PublicModules(basePath)})
	})
}

func (m *Modules) Register(module HTTPModule) error {
	if module == nil || !moduleName.MatchString(module.Name()) {
		return fmt.Errorf("invalid HTTP module name")
	}
	if _, exists := m.items[module.Name()]; exists {
		return fmt.Errorf("HTTP module %q is already registered", module.Name())
	}
	m.items[module.Name()] = module
	return nil
}

func (m *Modules) Handler() http.Handler {
	mux := http.NewServeMux()
	for name, module := range m.items {
		prefix := "/" + name
		handler := http.StripPrefix(prefix, module.Handler())
		mux.HandleFunc(prefix, func(w http.ResponseWriter, r *http.Request) {
			target := name + "/"
			if r.URL.RawQuery != "" {
				target += "?" + r.URL.RawQuery
			}
			http.Redirect(w, r, target, http.StatusPermanentRedirect)
		})
		mux.Handle(prefix+"/", handler)
	}
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json; charset=utf-8")
		w.WriteHeader(http.StatusNotFound)
		_ = json.NewEncoder(w).Encode(map[string]any{"error": map[string]string{
			"code": "api_module_not_found", "message": "API module or endpoint not found",
		}})
	})
	return mux
}

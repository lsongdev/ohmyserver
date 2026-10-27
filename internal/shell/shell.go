// Package shell is OhMyServer's panel host. It mounts modules that follow
// the module contract (docs/module-contract.md) and serves the shell UI
// around them: sign-in, navigation, and a shadow root per module.
//
// Modules never import this package: a module is any value with Handler and
// Assets methods, described by the manifest.json in its assets.
package shell

import (
	"encoding/json"
	"errors"
	"fmt"
	"html"
	"io/fs"
	"net/http"
	"path"
	"regexp"
	"strings"

	"github.com/lsongdev/ohmyserver/internal/shell/ui"
)

// ContractVersion is the module contract version this shell supports.
const ContractVersion = 1

// Module is satisfied implicitly by contract modules.
type Module interface {
	Handler() http.Handler // the module's API, with relative routes
	Assets() fs.FS         // browser assets, manifest.json at the root
}

// Manifest is a module's manifest.json.
type Manifest struct {
	Contract int       `json:"contract"`
	ID       string    `json:"id"`
	Title    string    `json:"title"`
	Icon     string    `json:"icon,omitempty"`
	Entry    string    `json:"entry"`
	Styles   []string  `json:"styles,omitempty"`
	Home     string    `json:"home,omitempty"`
	Nav      []NavItem `json:"nav,omitempty"`
	// Public lists API paths served without the panel's sign-in, such as
	// webhook endpoints; the module authenticates them itself. A path
	// ending in "/" covers everything below it.
	Public []string `json:"public,omitempty"`
}

type NavItem struct {
	Path  string `json:"path"`
	Title string `json:"title"`
	Icon  string `json:"icon,omitempty"`
	Group string `json:"group,omitempty"`
}

// Options configures a Host.
type Options struct {
	Title  string            // product name shown in the shell
	Lang   string            // interface language; "en" by default
	Auth   *Auth             // nil serves without authentication
	Home   string            // id of the module opened at "/"; the first by default
	Titles map[string]string // navigation titles overriding manifests, by manifest id
	// IDs mounts modules under ids of the panel's choosing instead of their
	// manifests' (e.g. "gateway" for yabber), by manifest id. The id names
	// the module's URLs and what ctx.link and Home refer to.
	IDs map[string]string
}

// Host serves modules inside the shell.
type Host struct {
	opts    Options
	modules []mounted
	index   []byte
	handler http.Handler
}

type mounted struct {
	Module
	manifest Manifest
}

var idPattern = regexp.MustCompile(`^[a-z][a-z0-9-]*$`)

// ReadManifest reads and validates a module's manifest.
func ReadManifest(m Module) (Manifest, error) {
	assets := m.Assets()
	data, err := fs.ReadFile(assets, "manifest.json")
	if err != nil {
		return Manifest{}, fmt.Errorf("manifest: %w", err)
	}
	var man Manifest
	if err := json.Unmarshal(data, &man); err != nil {
		return Manifest{}, fmt.Errorf("manifest.json: %w", err)
	}
	switch {
	case man.Contract != ContractVersion:
		return man, fmt.Errorf("module %q uses contract %d; this shell supports %d", man.ID, man.Contract, ContractVersion)
	case !idPattern.MatchString(man.ID):
		return man, fmt.Errorf("invalid module id %q", man.ID)
	case man.Title == "":
		return man, fmt.Errorf("module %q has no title", man.ID)
	}
	for _, name := range append([]string{man.Entry}, man.Styles...) {
		if _, err := fs.Stat(assets, strings.TrimPrefix(name, "/")); err != nil || name == "" {
			return man, fmt.Errorf("module %q: asset %q is missing", man.ID, name)
		}
	}
	for _, p := range man.Public {
		if !strings.HasPrefix(p, "/") || p == "/" || strings.ContainsAny(p, " {}") || path.Clean(p) != strings.TrimSuffix(p, "/") {
			return man, fmt.Errorf("module %q: invalid public path %q", man.ID, p)
		}
	}
	return man, nil
}

// New creates a host for the given modules.
func New(opts Options, modules ...Module) (*Host, error) {
	if len(modules) == 0 {
		return nil, errors.New("shell: at least one module is required")
	}
	if opts.Title == "" {
		opts.Title = "OhMyServer"
	}
	if opts.Lang == "" {
		opts.Lang = "en"
	}
	h := &Host{opts: opts}
	seen := map[string]bool{}
	for _, m := range modules {
		man, err := ReadManifest(m)
		if err != nil {
			return nil, fmt.Errorf("shell: %w", err)
		}
		if title := opts.Titles[man.ID]; title != "" {
			man.Title = title
		}
		if id := opts.IDs[man.ID]; id != "" {
			if !idPattern.MatchString(id) {
				return nil, fmt.Errorf("shell: invalid id %q for module %q", id, man.ID)
			}
			man.ID = id
		}
		if seen[man.ID] {
			return nil, fmt.Errorf("shell: duplicate module id %q", man.ID)
		}
		seen[man.ID] = true
		h.modules = append(h.modules, mounted{Module: m, manifest: man})
	}
	if opts.Home != "" && !seen[opts.Home] {
		return nil, fmt.Errorf("shell: home module %q is not mounted", opts.Home)
	}
	h.index = fmt.Appendf(nil, indexPage, html.EscapeString(opts.Lang), html.EscapeString(opts.Title))
	h.handler = h.routes()
	return h, nil
}

// Handler returns the host's HTTP handler.
func (h *Host) Handler() http.Handler { return h.handler }

// Close closes every module that implements io.Closer, in reverse order.
func (h *Host) Close() error {
	var errs []error
	for i := len(h.modules) - 1; i >= 0; i-- {
		if c, ok := h.modules[i].Module.(interface{ Close() error }); ok {
			errs = append(errs, c.Close())
		}
	}
	return errors.Join(errs...)
}

func (h *Host) routes() http.Handler {
	mux := http.NewServeMux()
	auth := h.opts.Auth
	protect := func(next http.Handler) http.Handler {
		if auth == nil {
			return next
		}
		return auth.require(next)
	}
	mux.HandleFunc("GET /{$}", h.serveIndex)
	mux.Handle("GET /_shell/", http.StripPrefix("/_shell/", staticHandler(ui.FS)))
	mux.Handle("GET /_panel.json", protect(http.HandlerFunc(h.serveManifest)))
	if auth != nil {
		auth.register(mux)
	} else {
		mux.HandleFunc("GET /_auth/session", func(w http.ResponseWriter, r *http.Request) {
			writeJSON(w, http.StatusOK, map[string]bool{"authenticated": true, "required": false})
		})
	}
	for _, m := range h.modules {
		base := "/m/" + m.manifest.ID
		mux.Handle("GET "+base+"/ui/", http.StripPrefix(base+"/ui", staticHandler(m.Assets())))
		api := http.StripPrefix(base+"/api", m.Handler())
		mux.Handle(base+"/api/", protect(api))
		for _, p := range m.manifest.Public {
			mux.Handle(base+"/api"+p, api) // the module authenticates these
		}
	}
	// Every state-changing request must come from the panel's own origin.
	return http.NewCrossOriginProtection().Handler(mux)
}

// moduleInfo is a mounted module as the shell UI sees it.
type moduleInfo struct {
	Manifest
	API string `json:"api"` // relative to the shell root
	UI  string `json:"ui"`
}

func (h *Host) serveManifest(w http.ResponseWriter, r *http.Request) {
	modules := make([]moduleInfo, 0, len(h.modules))
	for _, m := range h.modules {
		base := "m/" + m.manifest.ID + "/"
		modules = append(modules, moduleInfo{Manifest: m.manifest, API: base + "api/", UI: base + "ui/"})
	}
	home := h.opts.Home
	if home == "" {
		home = h.modules[0].manifest.ID
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"title": h.opts.Title, "lang": h.opts.Lang, "home": home, "auth": h.opts.Auth != nil, "modules": modules,
	})
}

const indexPage = `<!doctype html>
<html lang="%s">
<head>
  <meta charset="utf-8">
  <meta name="viewport" content="width=device-width, initial-scale=1">
  <meta name="color-scheme" content="light dark">
  <title>%s</title>
  <link rel="icon" href="data:image/svg+xml,%%3Csvg xmlns='http://www.w3.org/2000/svg' viewBox='0 0 16 16'%%3E%%3Cpath d='M8 1l7 7-7 7-7-7z' fill='%%232563eb'/%%3E%%3C/svg%%3E">
  <link rel="stylesheet" href="_shell/vendor/stylesheets.css">
  <link rel="stylesheet" href="_shell/style.css">
  <script type="module" src="_shell/shell.js"></script>
</head>
<body>
  <div id="app"></div>
  <noscript>This panel requires JavaScript.</noscript>
</body>
</html>
`

// csp allows only the panel's own scripts, styles, images and connections.
const csp = "default-src 'self'; script-src 'self'; img-src 'self' data:; style-src 'self' 'unsafe-inline'; " +
	"connect-src 'self'; object-src 'none'; base-uri 'self'; form-action 'self'; frame-ancestors 'none'"

func (h *Host) serveIndex(w http.ResponseWriter, r *http.Request) {
	hdr := w.Header()
	hdr.Set("Content-Type", "text/html; charset=utf-8")
	hdr.Set("Content-Security-Policy", csp)
	hdr.Set("Referrer-Policy", "same-origin")
	hdr.Set("X-Content-Type-Options", "nosniff")
	hdr.Set("Cache-Control", "no-cache")
	w.Write(h.index)
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(v)
}

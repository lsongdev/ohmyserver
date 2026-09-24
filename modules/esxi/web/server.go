package web

import (
	"encoding/json"
	"net/http"
	"strconv"
	"strings"

	esxi "github.com/lsongdev/ohmyserver/modules/esxi"
)

type DTO = esxi.DTO
type Input = esxi.Input
type Inventory = esxi.Inventory
type ESXiService = esxi.Service

type Module struct{ service ESXiService }

func New(s ESXiService) *Module { return &Module{service: s} }
func (*Module) Name() string    { return "esxi" }
func (m *Module) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /servers", m.list)
	mux.HandleFunc("POST /servers", m.create)
	mux.HandleFunc("GET /servers/{id}", m.get)
	mux.HandleFunc("PUT /servers/{id}", m.update)
	mux.HandleFunc("DELETE /servers/{id}", m.delete)
	mux.HandleFunc("POST /servers/{id}/test", m.test)
	mux.HandleFunc("GET /servers/{id}/inventory", m.inventory)
	return wrapModuleWeb(mux)
}
func (m *Module) StandaloneHandler() http.Handler {
	mux := http.NewServeMux()
	mux.Handle("/v1/esxi/", http.StripPrefix("/v1/esxi", m.Handler()))
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
func (m *Module) list(w http.ResponseWriter, r *http.Request) {
	v, e := m.service.ListESXi(r.Context())
	if e != nil {
		fail(w, 503, "esxi_unavailable", e.Error())
		return
	}
	if v == nil {
		v = []DTO{}
	}
	ok(w, 200, v)
}
func (m *Module) create(w http.ResponseWriter, r *http.Request) {
	var in Input
	if e := decode(w, r, &in); e != nil {
		fail(w, 400, "invalid_request", e.Error())
		return
	}
	if strings.TrimSpace(in.Name) == "" || strings.TrimSpace(in.Host) == "" || strings.TrimSpace(in.Username) == "" || in.Password == nil || *in.Password == "" {
		fail(w, 422, "validation_failed", "name, host, username and password are required")
		return
	}
	v, e := m.service.CreateESXi(r.Context(), in)
	if e != nil {
		fail(w, 409, "esxi_create_failed", e.Error())
		return
	}
	ok(w, 201, v)
}
func (m *Module) get(w http.ResponseWriter, r *http.Request) {
	id, g := idOf(w, r)
	if !g {
		return
	}
	v, e := m.service.GetESXi(r.Context(), id)
	if e != nil {
		fail(w, 404, "esxi_not_found", e.Error())
		return
	}
	ok(w, 200, v)
}
func (m *Module) update(w http.ResponseWriter, r *http.Request) {
	id, g := idOf(w, r)
	if !g {
		return
	}
	var in Input
	if e := decode(w, r, &in); e != nil {
		fail(w, 400, "invalid_request", e.Error())
		return
	}
	if strings.TrimSpace(in.Name) == "" || strings.TrimSpace(in.Host) == "" || strings.TrimSpace(in.Username) == "" {
		fail(w, 422, "validation_failed", "name, host and username are required")
		return
	}
	v, e := m.service.UpdateESXi(r.Context(), id, in)
	if e != nil {
		fail(w, 409, "esxi_update_failed", e.Error())
		return
	}
	ok(w, 200, v)
}
func (m *Module) delete(w http.ResponseWriter, r *http.Request) {
	id, g := idOf(w, r)
	if !g {
		return
	}
	if e := m.service.DeleteESXi(r.Context(), id); e != nil {
		fail(w, 404, "esxi_not_found", e.Error())
		return
	}
	w.WriteHeader(204)
}
func (m *Module) test(w http.ResponseWriter, r *http.Request) {
	id, g := idOf(w, r)
	if !g {
		return
	}
	if e := m.service.TestESXi(r.Context(), id); e != nil {
		fail(w, 502, "esxi_connection_failed", e.Error())
		return
	}
	ok(w, 200, map[string]bool{"connected": true})
}
func (m *Module) inventory(w http.ResponseWriter, r *http.Request) {
	id, g := idOf(w, r)
	if !g {
		return
	}
	v, e := m.service.ESXiInventory(r.Context(), id)
	if e != nil {
		fail(w, 502, "esxi_inventory_failed", e.Error())
		return
	}
	ok(w, 200, v)
}
func idOf(w http.ResponseWriter, r *http.Request) (int64, bool) {
	id, e := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if e != nil || id < 1 {
		fail(w, 400, "invalid_id", "invalid ESXi server id")
		return 0, false
	}
	return id, true
}
func decode(w http.ResponseWriter, r *http.Request, v any) error {
	d := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<20))
	d.DisallowUnknownFields()
	return d.Decode(v)
}
func ok(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(map[string]any{"data": v})
}
func fail(w http.ResponseWriter, status int, code, msg string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(map[string]any{"error": map[string]string{"code": code, "message": msg}})
}

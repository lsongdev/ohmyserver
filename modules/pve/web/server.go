package web

import (
	"encoding/json"
	"fmt"
	pvemodule "github.com/lsongdev/ohmyserver/modules/pve"
	"net/http"
	"strconv"
	"strings"
)

type DTO = pvemodule.DTO
type Input = pvemodule.Input
type VM = pvemodule.VM
type Node = pvemodule.Node
type Inventory = pvemodule.Inventory
type CreateVMInput = pvemodule.CreateVMInput
type CreateVMResult = pvemodule.CreateVMResult
type Service = pvemodule.Service
type Module struct{ service Service }

func New(s Service) *Module  { return &Module{service: s} }
func (*Module) Name() string { return "pve" }
func (m *Module) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /servers", m.list)
	mux.HandleFunc("POST /servers", m.create)
	mux.HandleFunc("GET /servers/{id}", m.get)
	mux.HandleFunc("PUT /servers/{id}", m.update)
	mux.HandleFunc("DELETE /servers/{id}", m.delete)
	mux.HandleFunc("POST /servers/{id}/test", m.test)
	mux.HandleFunc("GET /servers/{id}/inventory", m.inventory)
	mux.HandleFunc("POST /servers/{id}/vms", m.createVM)
	return wrapModuleWeb(mux)
}
func (m *Module) StandaloneHandler() http.Handler {
	mux := http.NewServeMux()
	mux.Handle("/v1/pve/", http.StripPrefix("/v1/pve", m.Handler()))
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
	v, e := m.service.ListPVE(r.Context())
	if e != nil {
		fail(w, 503, "pve_unavailable", e.Error())
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
	v, e := m.service.CreatePVE(r.Context(), in)
	if e != nil {
		fail(w, 409, "pve_create_failed", e.Error())
		return
	}
	ok(w, 201, v)
}
func (m *Module) get(w http.ResponseWriter, r *http.Request) {
	id, g := idOf(w, r)
	if !g {
		return
	}
	v, e := m.service.GetPVE(r.Context(), id)
	if e != nil {
		fail(w, 404, "pve_not_found", e.Error())
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
	v, e := m.service.UpdatePVE(r.Context(), id, in)
	if e != nil {
		fail(w, 409, "pve_update_failed", e.Error())
		return
	}
	ok(w, 200, v)
}
func (m *Module) delete(w http.ResponseWriter, r *http.Request) {
	id, g := idOf(w, r)
	if !g {
		return
	}
	if e := m.service.DeletePVE(r.Context(), id); e != nil {
		fail(w, 404, "pve_not_found", e.Error())
		return
	}
	w.WriteHeader(204)
}
func (m *Module) test(w http.ResponseWriter, r *http.Request) {
	id, g := idOf(w, r)
	if !g {
		return
	}
	if e := m.service.TestPVE(r.Context(), id); e != nil {
		fail(w, 502, "pve_connection_failed", e.Error())
		return
	}
	ok(w, 200, map[string]bool{"connected": true})
}
func (m *Module) inventory(w http.ResponseWriter, r *http.Request) {
	id, g := idOf(w, r)
	if !g {
		return
	}
	v, e := m.service.PVEInventory(r.Context(), id)
	if e != nil {
		fail(w, 502, "pve_inventory_failed", e.Error())
		return
	}
	ok(w, 200, v)
}
func (m *Module) createVM(w http.ResponseWriter, r *http.Request) {
	id, g := idOf(w, r)
	if !g {
		return
	}
	var in CreateVMInput
	if e := decode(w, r, &in); e != nil {
		fail(w, 400, "invalid_request", e.Error())
		return
	}
	if strings.TrimSpace(in.Node) == "" || in.TemplateID < 1 || strings.TrimSpace(in.Name) == "" {
		fail(w, 422, "validation_failed", "node, template_id and name are required")
		return
	}
	v, e := m.service.CreatePVEVM(r.Context(), id, in)
	if e != nil {
		fail(w, 502, "pve_vm_create_failed", e.Error())
		return
	}
	ok(w, 201, v)
}
func idOf(w http.ResponseWriter, r *http.Request) (int64, bool) {
	id, e := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if e != nil || id < 1 {
		fail(w, 400, "invalid_id", "invalid PVE server id")
		return 0, false
	}
	return id, true
}
func decode(w http.ResponseWriter, r *http.Request, v any) error {
	if !strings.HasPrefix(r.Header.Get("Content-Type"), "application/json") {
		return fmt.Errorf("content-type must be application/json")
	}
	d := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<20))
	d.DisallowUnknownFields()
	if err := d.Decode(v); err != nil {
		return err
	}
	return nil
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

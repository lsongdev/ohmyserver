package web

import (
	"encoding/json"
	ipmi "github.com/lsongdev/ohmyserver/modules/ipmi"
	"net/http"
	"strconv"
	"strings"
)

type ServerDTO = ipmi.ServerDTO
type CreateRequest = ipmi.CreateRequest
type UpdateRequest = ipmi.UpdateRequest
type PowerRequest = ipmi.PowerRequest
type HostInfo = ipmi.HostInfo
type FRU = ipmi.FRU
type Inventory = ipmi.Inventory
type Service = ipmi.Service

type Module struct{ service Service }

func New(service Service) *Module { return &Module{service: service} }
func (*Module) Name() string      { return "ipmi" }
func (m *Module) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /servers", m.list)
	mux.HandleFunc("POST /servers", m.create)
	mux.HandleFunc("GET /servers/{id}", m.get)
	mux.HandleFunc("PUT /servers/{id}", m.update)
	mux.HandleFunc("DELETE /servers/{id}", m.delete)
	mux.HandleFunc("POST /servers/{id}/test", m.test)
	mux.HandleFunc("GET /servers/{id}/inventory", m.inventory)
	mux.HandleFunc("POST /servers/{id}/power", m.power)
	return wrapModuleWeb(mux)
}
func (m *Module) StandaloneHandler() http.Handler {
	mux := http.NewServeMux()
	mux.Handle("/v1/ipmi/", http.StripPrefix("/v1/ipmi", m.Handler()))
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
	items, err := m.service.ListIPMIServers(r.Context())
	if err != nil {
		writeError(w, 503, "ipmi_unavailable", err.Error())
		return
	}
	if items == nil {
		items = []ServerDTO{}
	}
	writeJSON(w, 200, map[string]any{"data": items})
}
func (m *Module) create(w http.ResponseWriter, r *http.Request) {
	var in CreateRequest
	if err := decode(w, r, &in); err != nil {
		writeError(w, 400, "invalid_request", err.Error())
		return
	}
	if strings.TrimSpace(in.Name) == "" || strings.TrimSpace(in.BMCIP) == "" || strings.TrimSpace(in.Username) == "" || in.Password == "" {
		writeError(w, 422, "validation_failed", "name, bmc_ip, username and password are required")
		return
	}
	if in.Port == 0 {
		in.Port = 623
	}
	item, err := m.service.CreateIPMIServerAPI(r.Context(), in)
	if err != nil {
		writeError(w, 409, "ipmi_create_failed", err.Error())
		return
	}
	writeJSON(w, 201, map[string]any{"data": item})
}
func (m *Module) get(w http.ResponseWriter, r *http.Request) {
	id, ok := parseID(w, r)
	if !ok {
		return
	}
	item, err := m.service.ReadIPMIServer(r.Context(), id)
	if err != nil {
		writeError(w, 404, "ipmi_not_found", err.Error())
		return
	}
	writeJSON(w, 200, map[string]any{"data": item})
}
func (m *Module) update(w http.ResponseWriter, r *http.Request) {
	id, ok := parseID(w, r)
	if !ok {
		return
	}
	var in UpdateRequest
	if err := decode(w, r, &in); err != nil {
		writeError(w, 400, "invalid_request", err.Error())
		return
	}
	if strings.TrimSpace(in.Name) == "" || strings.TrimSpace(in.BMCIP) == "" || strings.TrimSpace(in.Username) == "" {
		writeError(w, 422, "validation_failed", "name, bmc_ip and username are required")
		return
	}
	if in.Port == 0 {
		in.Port = 623
	}
	item, err := m.service.UpdateIPMIServerAPI(r.Context(), id, in)
	if err != nil {
		writeError(w, 409, "ipmi_update_failed", err.Error())
		return
	}
	writeJSON(w, 200, map[string]any{"data": item})
}
func (m *Module) delete(w http.ResponseWriter, r *http.Request) {
	id, ok := parseID(w, r)
	if !ok {
		return
	}
	if err := m.service.RemoveIPMIServerAPI(r.Context(), id); err != nil {
		writeError(w, 404, "ipmi_not_found", err.Error())
		return
	}
	w.WriteHeader(204)
}
func (m *Module) test(w http.ResponseWriter, r *http.Request) {
	id, ok := parseID(w, r)
	if !ok {
		return
	}
	if err := m.service.TestIPMIConnection(r.Context(), id); err != nil {
		writeError(w, 502, "ipmi_connection_failed", err.Error())
		return
	}
	writeJSON(w, 200, map[string]any{"data": map[string]any{"connected": true}})
}
func (m *Module) inventory(w http.ResponseWriter, r *http.Request) {
	id, ok := parseID(w, r)
	if !ok {
		return
	}
	item, err := m.service.IPMIInventory(r.Context(), id)
	if err != nil {
		writeError(w, 502, "ipmi_inventory_failed", err.Error())
		return
	}
	writeJSON(w, 200, map[string]any{"data": item})
}
func (m *Module) power(w http.ResponseWriter, r *http.Request) {
	id, ok := parseID(w, r)
	if !ok {
		return
	}
	var in PowerRequest
	if err := decode(w, r, &in); err != nil {
		writeError(w, 400, "invalid_request", err.Error())
		return
	}
	switch in.Action {
	case "on", "off", "cycle", "reset":
	default:
		writeError(w, 422, "invalid_action", "action must be on, off, cycle, or reset")
		return
	}
	if err := m.service.ControlIPMIPower(r.Context(), id, in.Action); err != nil {
		writeError(w, 502, "ipmi_power_failed", err.Error())
		return
	}
	w.WriteHeader(204)
}
func parseID(w http.ResponseWriter, r *http.Request) (int64, bool) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil || id < 1 {
		writeError(w, 400, "invalid_id", "invalid IPMI server id")
		return 0, false
	}
	return id, true
}
func decode(w http.ResponseWriter, r *http.Request, out any) error {
	if !strings.HasPrefix(r.Header.Get("Content-Type"), "application/json") {
		return http.ErrNotSupported
	}
	r.Body = http.MaxBytesReader(w, r.Body, 64<<10)
	d := json.NewDecoder(r.Body)
	d.DisallowUnknownFields()
	return d.Decode(out)
}
func writeJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}
func writeError(w http.ResponseWriter, status int, code, message string) {
	writeJSON(w, status, map[string]any{"error": map[string]string{"code": code, "message": message}})
}

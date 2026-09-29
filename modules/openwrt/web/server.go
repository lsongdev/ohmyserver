package web

import (
	"context"
	"encoding/json"
	"errors"
	openwrtmodule "github.com/lsongdev/ohmyserver/modules/openwrt"
	"net/http"
	"strconv"
	"strings"
)

type Runtime interface {
	ReadOpenWrt(context.Context, int64, string) (any, error)
	TestOpenWrt(context.Context, int64) error
	ActOpenWrt(context.Context, int64, string, string) error
}

type Module struct {
	repo    *Repository
	runtime Runtime
}

func New(repo *Repository, runtime Runtime) *Module { return &Module{repo: repo, runtime: runtime} }
func (*Module) Name() string                        { return "openwrt" }

func (m *Module) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /routers", m.list)
	mux.HandleFunc("POST /routers", m.create)
	mux.HandleFunc("GET /routers/{id}", m.get)
	mux.HandleFunc("PUT /routers/{id}", m.update)
	mux.HandleFunc("DELETE /routers/{id}", m.delete)
	mux.HandleFunc("POST /routers/{id}/test", m.test)
	mux.HandleFunc("GET /routers/{id}/{section}", m.read)
	mux.HandleFunc("POST /routers/{id}/actions", m.action)
	return wrapModuleWeb(mux)
}

type input struct {
	Name        string `json:"name"`
	Host        string `json:"host"`
	Port        int    `json:"port"`
	Username    string `json:"username"`
	Password    string `json:"password"`
	Protocol    string `json:"protocol"`
	Description string `json:"description"`
}

func (m *Module) list(w http.ResponseWriter, r *http.Request) {
	items, err := m.repo.List(r.Context())
	if err != nil {
		fail(w, 500, "router_list_failed", err.Error())
		return
	}
	ok(w, 200, items)
}

func (m *Module) create(w http.ResponseWriter, r *http.Request) {
	var in input
	if err := decode(w, r, &in); err != nil {
		fail(w, 400, "invalid_request", err.Error())
		return
	}
	normalizeInput(&in)
	if err := validate(in); err != nil {
		fail(w, 422, "validation_failed", err.Error())
		return
	}
	v := &openwrtmodule.Server{Name: strings.TrimSpace(in.Name), Host: strings.TrimSpace(in.Host), Port: in.Port, Username: in.Username, Password: in.Password, Protocol: in.Protocol, Description: in.Description}
	if err := m.repo.Create(r.Context(), v); err != nil {
		fail(w, 409, "router_create_failed", err.Error())
		return
	}
	ok(w, 201, v)
}

func (m *Module) get(w http.ResponseWriter, r *http.Request) {
	id, good := routerID(w, r)
	if !good {
		return
	}
	v, err := m.repo.Get(r.Context(), id)
	if err != nil {
		fail(w, 404, "router_not_found", err.Error())
		return
	}
	ok(w, 200, v)
}

func (m *Module) update(w http.ResponseWriter, r *http.Request) {
	id, good := routerID(w, r)
	if !good {
		return
	}
	var in input
	if err := decode(w, r, &in); err != nil {
		fail(w, 400, "invalid_request", err.Error())
		return
	}
	normalizeInput(&in)
	if err := validate(in); err != nil {
		fail(w, 422, "validation_failed", err.Error())
		return
	}
	v, err := m.repo.Get(r.Context(), id)
	if err != nil {
		fail(w, 404, "router_not_found", err.Error())
		return
	}
	v.Name, v.Host, v.Port, v.Username, v.Protocol, v.Description = strings.TrimSpace(in.Name), strings.TrimSpace(in.Host), in.Port, in.Username, in.Protocol, in.Description
	if in.Password != "" {
		v.Password = in.Password
	}
	if err := m.repo.Update(r.Context(), v); err != nil {
		fail(w, 409, "router_update_failed", err.Error())
		return
	}
	ok(w, 200, v)
}

func (m *Module) delete(w http.ResponseWriter, r *http.Request) {
	id, good := routerID(w, r)
	if !good {
		return
	}
	if err := m.repo.Delete(r.Context(), id); err != nil {
		fail(w, 404, "router_not_found", err.Error())
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (m *Module) test(w http.ResponseWriter, r *http.Request) {
	id, good := routerID(w, r)
	if !good {
		return
	}
	if err := m.runtime.TestOpenWrt(r.Context(), id); err != nil {
		fail(w, 502, "router_unavailable", err.Error())
		return
	}
	ok(w, 200, map[string]bool{"connected": true})
}

func (m *Module) read(w http.ResponseWriter, r *http.Request) {
	id, good := routerID(w, r)
	if !good {
		return
	}
	section := r.PathValue("section")
	switch section {
	case "overview", "network", "dhcp", "packages", "firewall", "services":
	default:
		fail(w, 404, "section_not_found", "unknown OpenWrt section")
		return
	}
	v, err := m.runtime.ReadOpenWrt(r.Context(), id, section)
	if err != nil {
		fail(w, 502, "router_read_failed", err.Error())
		return
	}
	ok(w, 200, v)
}

type actionInput struct {
	Action  string `json:"action"`
	Service string `json:"service"`
}

func (m *Module) action(w http.ResponseWriter, r *http.Request) {
	id, good := routerID(w, r)
	if !good {
		return
	}
	var in actionInput
	if err := decode(w, r, &in); err != nil {
		fail(w, 400, "invalid_request", err.Error())
		return
	}
	if in.Action != "reboot" && in.Action != "restart_service" {
		fail(w, 422, "validation_failed", "unsupported action")
		return
	}
	if in.Action == "restart_service" && strings.TrimSpace(in.Service) == "" {
		fail(w, 422, "validation_failed", "service is required")
		return
	}
	if err := m.runtime.ActOpenWrt(r.Context(), id, in.Action, in.Service); err != nil {
		fail(w, 502, "router_action_failed", err.Error())
		return
	}
	ok(w, 200, map[string]string{"state": "accepted"})
}

func validate(in input) error {
	if strings.TrimSpace(in.Name) == "" || strings.TrimSpace(in.Host) == "" || strings.TrimSpace(in.Username) == "" {
		return errors.New("name, host and username are required")
	}
	if in.Port < 1 || in.Port > 65535 {
		return errors.New("port must be between 1 and 65535")
	}
	if in.Protocol != "http" && in.Protocol != "https" {
		return errors.New("protocol must be http or https")
	}
	return nil
}

func normalizeInput(in *input) {
	if in.Protocol == "" {
		in.Protocol = "http"
	}
	if in.Port == 0 {
		in.Port = 80
		if in.Protocol == "https" {
			in.Port = 443
		}
	}
}
func routerID(w http.ResponseWriter, r *http.Request) (int64, bool) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil || id < 1 {
		fail(w, 400, "invalid_id", "invalid router id")
		return 0, false
	}
	return id, true
}
func decode(w http.ResponseWriter, r *http.Request, value any) error {
	d := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<20))
	d.DisallowUnknownFields()
	return d.Decode(value)
}
func ok(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(map[string]any{"data": value})
}
func fail(w http.ResponseWriter, status int, code, message string) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(map[string]any{"error": map[string]string{"code": code, "message": message}})
}

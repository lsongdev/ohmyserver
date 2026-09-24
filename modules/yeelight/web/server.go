package web

import (
	"encoding/hex"
	"encoding/json"
	"net/http"
	"strconv"
	"strings"

	yeelight "github.com/lsongdev/ohmyserver/modules/yeelight"
)

type Input = yeelight.Input
type State = yeelight.State
type FoundDevice = yeelight.FoundDevice
type Capability = yeelight.Service
type Control = yeelight.Control

type Module struct{ service Capability }

func New(s Capability) *Module { return &Module{service: s} }
func (*Module) Name() string   { return "yeelight" }
func (m *Module) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /devices", m.list)
	mux.HandleFunc("POST /devices", m.create)
	mux.HandleFunc("GET /devices/{id}", m.get)
	mux.HandleFunc("PUT /devices/{id}", m.update)
	mux.HandleFunc("DELETE /devices/{id}", m.delete)
	mux.HandleFunc("GET /devices/{id}/state", m.state)
	mux.HandleFunc("POST /devices/{id}/test", m.test)
	mux.HandleFunc("POST /devices/{id}/control", m.control)
	mux.HandleFunc("GET /discovery", m.discover)
	return wrapModuleWeb(mux)
}
func (m *Module) StandaloneHandler() http.Handler {
	mux := http.NewServeMux()
	mux.Handle("/v1/yeelight/", http.StripPrefix("/v1/yeelight", m.Handler()))
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
	v, e := m.service.ListYeelight(r.Context())
	if e != nil {
		fail(w, 503, "yeelight_unavailable", e.Error())
		return
	}
	if v == nil {
		v = []Device{}
	}
	ok(w, 200, v)
}
func (m *Module) create(w http.ResponseWriter, r *http.Request) {
	var in Input
	if e := decode(w, r, &in); e != nil {
		fail(w, 400, "invalid_request", e.Error())
		return
	}
	in.Host = strings.TrimSpace(in.Host)
	in.Name = strings.TrimSpace(in.Name)
	if in.Host == "" {
		fail(w, 422, "validation_failed", "host is required")
		return
	}
	if in.Port == 0 {
		in.Port = 55443
	}
	if in.Port < 1 || in.Port > 65535 {
		fail(w, 422, "validation_failed", "port must be between 1 and 65535")
		return
	}
	if in.Name == "" {
		in.Name = in.Host
	}
	v, e := m.service.CreateYeelight(r.Context(), in)
	if e != nil {
		fail(w, 409, "device_create_failed", e.Error())
		return
	}
	ok(w, 201, v)
}
func (m *Module) get(w http.ResponseWriter, r *http.Request) {
	id, g := idOf(w, r)
	if !g {
		return
	}
	v, e := m.service.GetYeelight(r.Context(), id)
	if e != nil {
		fail(w, 404, "device_not_found", e.Error())
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
	in.Name = strings.TrimSpace(in.Name)
	in.Host = strings.TrimSpace(in.Host)
	if in.Name == "" || in.Host == "" {
		fail(w, 422, "validation_failed", "name and host are required")
		return
	}
	if in.Port == 0 {
		in.Port = 55443
	}
	if in.Port < 1 || in.Port > 65535 {
		fail(w, 422, "validation_failed", "port must be between 1 and 65535")
		return
	}
	v, e := m.service.UpdateYeelight(r.Context(), id, in)
	if e != nil {
		fail(w, 409, "device_update_failed", e.Error())
		return
	}
	ok(w, 200, v)
}
func (m *Module) delete(w http.ResponseWriter, r *http.Request) {
	id, g := idOf(w, r)
	if !g {
		return
	}
	if e := m.service.DeleteYeelight(r.Context(), id); e != nil {
		fail(w, 404, "device_not_found", e.Error())
		return
	}
	w.WriteHeader(204)
}
func (m *Module) state(w http.ResponseWriter, r *http.Request) {
	id, g := idOf(w, r)
	if !g {
		return
	}
	v, e := m.service.YeelightState(r.Context(), id)
	if e != nil {
		fail(w, 502, "device_unavailable", e.Error())
		return
	}
	ok(w, 200, v)
}
func (m *Module) test(w http.ResponseWriter, r *http.Request) {
	id, g := idOf(w, r)
	if !g {
		return
	}
	if e := m.service.TestYeelight(r.Context(), id); e != nil {
		fail(w, 502, "device_unavailable", e.Error())
		return
	}
	ok(w, 200, map[string]bool{"connected": true})
}

func (m *Module) control(w http.ResponseWriter, r *http.Request) {
	id, g := idOf(w, r)
	if !g {
		return
	}
	var in Control
	if e := decode(w, r, &in); e != nil {
		fail(w, 400, "invalid_request", e.Error())
		return
	}
	switch in.Action {
	case "on", "off", "toggle":
	case "brightness":
		if in.Value < 1 || in.Value > 100 {
			fail(w, 422, "validation_failed", "brightness must be between 1 and 100")
			return
		}
	case "color_temperature":
		if in.Value < 1700 || in.Value > 6500 {
			fail(w, 422, "validation_failed", "color temperature must be between 1700 and 6500")
			return
		}
	case "color":
		color := strings.TrimPrefix(in.Color, "#")
		if len(color) != 6 {
			fail(w, 422, "validation_failed", "color must be a six-digit RGB hex value")
			return
		}
		if _, err := hex.DecodeString(color); err != nil {
			fail(w, 422, "validation_failed", "color must be a six-digit RGB hex value")
			return
		}
	default:
		fail(w, 422, "validation_failed", "unsupported action")
		return
	}
	v, e := m.service.ControlYeelight(r.Context(), id, in.Action, in.Value, in.Color)
	if e != nil {
		fail(w, 502, "device_control_failed", e.Error())
		return
	}
	ok(w, 200, v)
}
func (m *Module) discover(w http.ResponseWriter, r *http.Request) {
	v, e := m.service.DiscoverYeelight(r.Context())
	if e != nil {
		fail(w, 502, "discovery_failed", e.Error())
		return
	}
	if v == nil {
		v = []FoundDevice{}
	}
	ok(w, 200, v)
}
func idOf(w http.ResponseWriter, r *http.Request) (int64, bool) {
	id, e := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if e != nil || id < 1 {
		fail(w, 400, "invalid_id", "invalid device id")
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

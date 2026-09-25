package web

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"mime"
	"net/http"
	"strconv"
	"strings"
	"time"
)

type DTO struct {
	ID          int64     `json:"id"`
	Name        string    `json:"name"`
	APIURL      string    `json:"api_url"`
	Status      string    `json:"status"`
	Description string    `json:"description"`
	HasSecret   bool      `json:"has_secret"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
}
type Input struct {
	Name        string  `json:"name"`
	APIURL      string  `json:"api_url"`
	Secret      *string `json:"secret"`
	Description string  `json:"description"`
}
type Service interface {
	ListClash(context.Context) ([]DTO, error)
	CreateClash(context.Context, Input) (DTO, error)
	GetClash(context.Context, int64) (DTO, error)
	UpdateClash(context.Context, int64, Input) (DTO, error)
	DeleteClash(context.Context, int64) error
	TestClash(context.Context, int64) error
	ClashProxies(context.Context, int64) (any, error)
	ClashRules(context.Context, int64) (any, error)
	ClashConnections(context.Context, int64) (any, error)
	ClashConfig(context.Context, int64) (any, error)
	ClashAction(context.Context, int64, Action) (any, error)
	UpdateClashConfig(context.Context, int64, map[string]any) (any, error)
	OpenClashStream(context.Context, int64) (Stream, error)
}
type Stream interface {
	Next(context.Context) (string, any, error)
	Close()
}
type Action struct {
	Action    string `json:"action"`
	Selector  string `json:"selector"`
	ProxyName string `json:"proxy_name"`
	Mode      string `json:"mode"`
	ID        string `json:"id"`
}
type Module struct{ service Service }

func New(s Service) *Module  { return &Module{service: s} }
func (*Module) Name() string { return "clash" }
func (m *Module) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /servers", m.list)
	mux.HandleFunc("POST /servers", m.create)
	mux.HandleFunc("GET /servers/{id}", m.get)
	mux.HandleFunc("PUT /servers/{id}", m.update)
	mux.HandleFunc("DELETE /servers/{id}", m.delete)
	mux.HandleFunc("POST /servers/{id}/test", m.test)
	mux.HandleFunc("GET /servers/{id}/proxies", m.proxies)
	mux.HandleFunc("GET /servers/{id}/rules", m.rules)
	mux.HandleFunc("GET /servers/{id}/connections", m.connections)
	mux.HandleFunc("GET /servers/{id}/config", m.config)
	mux.HandleFunc("PATCH /servers/{id}/config", m.updateConfig)
	mux.HandleFunc("POST /servers/{id}/actions", m.action)
	mux.HandleFunc("GET /servers/{id}/events", m.events)
	return wrapModuleWeb(mux)
}
func (m *Module) list(w http.ResponseWriter, r *http.Request) {
	v, e := m.service.ListClash(r.Context())
	if e != nil {
		fail(w, 503, "clash_unavailable", e.Error())
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
	if strings.TrimSpace(in.Name) == "" || strings.TrimSpace(in.APIURL) == "" {
		fail(w, 422, "validation_failed", "name and api_url are required")
		return
	}
	v, e := m.service.CreateClash(r.Context(), in)
	if e != nil {
		fail(w, 409, "clash_create_failed", e.Error())
		return
	}
	ok(w, 201, v)
}
func (m *Module) get(w http.ResponseWriter, r *http.Request) {
	id, g := idOf(w, r)
	if !g {
		return
	}
	v, e := m.service.GetClash(r.Context(), id)
	if e != nil {
		fail(w, 404, "clash_not_found", e.Error())
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
	if strings.TrimSpace(in.Name) == "" || strings.TrimSpace(in.APIURL) == "" {
		fail(w, 422, "validation_failed", "name and api_url are required")
		return
	}
	v, e := m.service.UpdateClash(r.Context(), id, in)
	if e != nil {
		fail(w, 409, "clash_update_failed", e.Error())
		return
	}
	ok(w, 200, v)
}
func (m *Module) delete(w http.ResponseWriter, r *http.Request) {
	id, g := idOf(w, r)
	if !g {
		return
	}
	if e := m.service.DeleteClash(r.Context(), id); e != nil {
		fail(w, 404, "clash_not_found", e.Error())
		return
	}
	w.WriteHeader(204)
}
func (m *Module) test(w http.ResponseWriter, r *http.Request) {
	id, g := idOf(w, r)
	if !g {
		return
	}
	if e := m.service.TestClash(r.Context(), id); e != nil {
		fail(w, 502, "clash_connection_failed", e.Error())
		return
	}
	ok(w, 200, map[string]bool{"connected": true})
}
func (m *Module) proxies(w http.ResponseWriter, r *http.Request) {
	id, g := idOf(w, r)
	if !g {
		return
	}
	v, e := m.service.ClashProxies(r.Context(), id)
	if e != nil {
		fail(w, 502, "clash_proxies_failed", e.Error())
		return
	}
	ok(w, 200, v)
}
func (m *Module) rules(w http.ResponseWriter, r *http.Request) {
	id, g := idOf(w, r)
	if !g {
		return
	}
	v, e := m.service.ClashRules(r.Context(), id)
	if e != nil {
		fail(w, 502, "clash_rules_failed", e.Error())
		return
	}
	ok(w, 200, v)
}
func (m *Module) connections(w http.ResponseWriter, r *http.Request) {
	id, g := idOf(w, r)
	if !g {
		return
	}
	v, e := m.service.ClashConnections(r.Context(), id)
	if e != nil {
		fail(w, 502, "clash_connections_failed", e.Error())
		return
	}
	ok(w, 200, v)
}
func (m *Module) config(w http.ResponseWriter, r *http.Request) {
	id, g := idOf(w, r)
	if !g {
		return
	}
	v, e := m.service.ClashConfig(r.Context(), id)
	if e != nil {
		fail(w, 502, "clash_config_failed", e.Error())
		return
	}
	ok(w, 200, v)
}
func (m *Module) updateConfig(w http.ResponseWriter, r *http.Request) {
	id, g := idOf(w, r)
	if !g {
		return
	}
	var in map[string]any
	if e := decode(w, r, &in); e != nil {
		fail(w, 400, "invalid_request", e.Error())
		return
	}
	v, e := m.service.UpdateClashConfig(r.Context(), id, in)
	if e != nil {
		fail(w, 422, "clash_config_update_failed", e.Error())
		return
	}
	ok(w, 200, v)
}
func (m *Module) action(w http.ResponseWriter, r *http.Request) {
	id, g := idOf(w, r)
	if !g {
		return
	}
	var in Action
	if e := decode(w, r, &in); e != nil {
		fail(w, 400, "invalid_request", e.Error())
		return
	}
	v, e := m.service.ClashAction(r.Context(), id, in)
	if e != nil {
		fail(w, 422, "clash_action_failed", e.Error())
		return
	}
	ok(w, 200, v)
}
func (m *Module) events(w http.ResponseWriter, r *http.Request) {
	id, g := idOf(w, r)
	if !g {
		return
	}
	stream, e := m.service.OpenClashStream(r.Context(), id)
	if e != nil {
		fail(w, 502, "clash_stream_failed", e.Error())
		return
	}
	defer stream.Close()
	f, ok := w.(http.Flusher)
	if !ok {
		fail(w, 500, "stream_unsupported", "streaming is not supported")
		return
	}
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("X-Accel-Buffering", "no")
	w.WriteHeader(200)
	f.Flush()
	for {
		event, data, err := stream.Next(r.Context())
		if err != nil {
			if r.Context().Err() != nil {
				return
			}
			payload, _ := json.Marshal(map[string]string{"message": err.Error()})
			if _, err = w.Write([]byte("event: error\ndata: " + string(payload) + "\n\n")); err != nil {
				return
			}
			f.Flush()
			return
		}
		payload, err := json.Marshal(data)
		if err != nil {
			continue
		}
		if _, err = w.Write([]byte("event: " + event + "\ndata: " + string(payload) + "\n\n")); err != nil {
			return
		}
		f.Flush()
	}
}
func idOf(w http.ResponseWriter, r *http.Request) (int64, bool) {
	id, e := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if e != nil || id < 1 {
		fail(w, 400, "invalid_id", "invalid Clash server id")
		return 0, false
	}
	return id, true
}
func decode(w http.ResponseWriter, r *http.Request, v any) error {
	mediaType, _, err := mime.ParseMediaType(r.Header.Get("Content-Type"))
	if err != nil || mediaType != "application/json" {
		return fmt.Errorf("Content-Type must be application/json")
	}
	d := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<20))
	d.DisallowUnknownFields()
	if err := d.Decode(v); err != nil {
		return err
	}
	var trailing any
	if err := d.Decode(&trailing); !errors.Is(err, io.EOF) {
		if err == nil {
			return fmt.Errorf("request body must contain one JSON value")
		}
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

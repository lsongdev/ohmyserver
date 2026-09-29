package web

import (
	"encoding/json"
	"net/http"
	"strconv"

	webservermodule "github.com/lsongdev/ohmyserver/modules/webserver"
)

type Listener = webservermodule.Listener
type Host = webservermodule.Host
type Route = webservermodule.Route
type Policy = webservermodule.Policy
type Certificate = webservermodule.Certificate
type Service = webservermodule.Service

type Module struct{ service Service }

func New(service Service) *Module { return &Module{service: service} }
func (*Module) Name() string      { return "webserver" }

func (m *Module) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /listeners", func(w http.ResponseWriter, r *http.Request) {
		items, err := m.service.WebListeners(r.Context())
		if err != nil {
			problem(w, err)
			return
		}
		respond(w, items)
	})
	mux.HandleFunc("GET /hosts", func(w http.ResponseWriter, r *http.Request) {
		items, err := m.service.WebHosts(r.Context())
		if err != nil {
			problem(w, err)
			return
		}
		respond(w, items)
	})
	mux.HandleFunc("GET /policies", func(w http.ResponseWriter, r *http.Request) {
		items, err := m.service.WebPolicies(r.Context())
		if err != nil {
			problem(w, err)
			return
		}
		respond(w, items)
	})
	mux.HandleFunc("GET /certificates", func(w http.ResponseWriter, r *http.Request) {
		items, err := m.service.WebCertificates(r.Context())
		if err != nil {
			problem(w, err)
			return
		}
		respond(w, items)
	})
	mux.HandleFunc("POST /listeners", func(w http.ResponseWriter, r *http.Request) {
		var item Listener
		if json.NewDecoder(r.Body).Decode(&item) != nil || item.Name == "" || item.Address == "" || item.Protocol == "" {
			problemStatus(w, 400, "invalid_listener", "name, address and protocol are required")
			return
		}
		created, err := m.service.CreateWebListenerAPI(r.Context(), item)
		if err != nil {
			problem(w, err)
			return
		}
		respondCreated(w, created)
	})
	mux.HandleFunc("PUT /listeners/{id}", func(w http.ResponseWriter, r *http.Request) {
		id, err := webID(r)
		if err != nil {
			problemStatus(w, 400, "invalid_id", "listener id must be positive")
			return
		}
		var item Listener
		if json.NewDecoder(r.Body).Decode(&item) != nil || item.Name == "" || item.Address == "" || item.Protocol == "" {
			problemStatus(w, 400, "invalid_listener", "name, address and protocol are required")
			return
		}
		if err := m.service.UpdateWebListenerAPI(r.Context(), id, item); err != nil {
			problem(w, err)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	})
	mux.HandleFunc("DELETE /listeners/{id}", func(w http.ResponseWriter, r *http.Request) {
		id, err := webID(r)
		if err != nil {
			problemStatus(w, 400, "invalid_id", "listener id must be positive")
			return
		}
		if err := m.service.DeleteWebListenerAPI(r.Context(), id); err != nil {
			problem(w, err)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	})
	mux.HandleFunc("POST /hosts", func(w http.ResponseWriter, r *http.Request) {
		var x Host
		if decodeJSON(r, &x) != nil || x.Name == "" || x.Hostnames == "" {
			problemStatus(w, 400, "invalid_host", "name and hostnames are required")
			return
		}
		result, err := m.service.CreateWebHostAPI(r.Context(), x)
		if err != nil {
			problem(w, err)
			return
		}
		respondCreated(w, result)
	})
	mux.HandleFunc("PUT /hosts/{id}", func(w http.ResponseWriter, r *http.Request) {
		id, err := webID(r)
		if err != nil {
			problemStatus(w, 400, "invalid_id", "host id must be positive")
			return
		}
		var x Host
		if decodeJSON(r, &x) != nil || x.Name == "" || x.Hostnames == "" {
			problemStatus(w, 400, "invalid_host", "name and hostnames are required")
			return
		}
		if err := m.service.UpdateWebHostAPI(r.Context(), id, x); err != nil {
			problem(w, err)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	})
	mux.HandleFunc("DELETE /hosts/{id}", func(w http.ResponseWriter, r *http.Request) {
		id, err := webID(r)
		if err != nil {
			problemStatus(w, 400, "invalid_id", "host id must be positive")
			return
		}
		if err := m.service.DeleteWebHostAPI(r.Context(), id); err != nil {
			problem(w, err)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	})
	mux.HandleFunc("POST /hosts/{id}/routes", func(w http.ResponseWriter, r *http.Request) {
		id, err := webID(r)
		if err != nil {
			problemStatus(w, 400, "invalid_id", "host id must be positive")
			return
		}
		var x Route
		if decodeJSON(r, &x) != nil || x.Name == "" || x.Path == "" || x.HandlerType == "" {
			problemStatus(w, 400, "invalid_route", "name, path and handler_type are required")
			return
		}
		x.HostID = id
		result, err := m.service.CreateWebRouteAPI(r.Context(), id, x)
		if err != nil {
			problem(w, err)
			return
		}
		respondCreated(w, result)
	})
	mux.HandleFunc("PUT /routes/{id}", func(w http.ResponseWriter, r *http.Request) {
		id, err := webID(r)
		if err != nil {
			problemStatus(w, 400, "invalid_id", "route id must be positive")
			return
		}
		var x Route
		if decodeJSON(r, &x) != nil || x.Name == "" || x.Path == "" || x.HandlerType == "" {
			problemStatus(w, 400, "invalid_route", "name, path and handler_type are required")
			return
		}
		x.ID = id
		if err := m.service.UpdateWebRouteAPI(r.Context(), id, x); err != nil {
			problem(w, err)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	})
	mux.HandleFunc("DELETE /routes/{id}", func(w http.ResponseWriter, r *http.Request) {
		id, err := webID(r)
		if err != nil {
			problemStatus(w, 400, "invalid_id", "route id must be positive")
			return
		}
		if err := m.service.DeleteWebRouteAPI(r.Context(), id); err != nil {
			problem(w, err)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	})
	mux.HandleFunc("POST /policies", func(w http.ResponseWriter, r *http.Request) {
		var x Policy
		if decodeJSON(r, &x) != nil || x.Name == "" {
			problemStatus(w, 400, "invalid_policy", "policy name is required")
			return
		}
		result, err := m.service.CreateWebPolicyAPI(r.Context(), x)
		if err != nil {
			problem(w, err)
			return
		}
		respondCreated(w, result)
	})
	mux.HandleFunc("PUT /policies/{id}", func(w http.ResponseWriter, r *http.Request) {
		id, err := webID(r)
		if err != nil {
			problemStatus(w, 400, "invalid_id", "policy id must be positive")
			return
		}
		var x Policy
		if decodeJSON(r, &x) != nil || x.Name == "" {
			problemStatus(w, 400, "invalid_policy", "policy name is required")
			return
		}
		if err := m.service.UpdateWebPolicyAPI(r.Context(), id, x); err != nil {
			problem(w, err)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	})
	mux.HandleFunc("DELETE /policies/{id}", func(w http.ResponseWriter, r *http.Request) {
		id, err := webID(r)
		if err != nil {
			problemStatus(w, 400, "invalid_id", "policy id must be positive")
			return
		}
		if err := m.service.DeleteWebPolicyAPI(r.Context(), id); err != nil {
			problem(w, err)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	})
	return wrapModuleWeb(mux)
}
func decodeJSON(r *http.Request, v any) error { return json.NewDecoder(r.Body).Decode(v) }
func respond(w http.ResponseWriter, items any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	_ = json.NewEncoder(w).Encode(map[string]any{"data": items})
}
func respondCreated(w http.ResponseWriter, item any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(http.StatusCreated)
	_ = json.NewEncoder(w).Encode(map[string]any{"data": item})
}
func webID(r *http.Request) (int64, error) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil || id < 1 {
		return 0, http.ErrMissingFile
	}
	return id, nil
}
func problemStatus(w http.ResponseWriter, status int, code, message string) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(map[string]any{"error": map[string]string{"code": code, "message": message}})
}
func problem(w http.ResponseWriter, err error) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(503)
	_ = json.NewEncoder(w).Encode(map[string]any{"error": map[string]string{"code": "webserver_unavailable", "message": err.Error()}})
}

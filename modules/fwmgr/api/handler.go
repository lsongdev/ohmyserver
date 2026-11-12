package api

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/netip"
	"strings"

	"github.com/lsongdev/ohmyserver/modules/fwmgr/system"
)

// Info describes what this machine can do.
type Info struct {
	Supported  bool `json:"supported"`
	Available  bool `json:"available"`  // nft is installed
	Forwarding bool `json:"forwarding"` // the kernel forwards IPv4 now
}

// ConfigView is the settings page's data.
type ConfigView struct {
	Config system.Config `json:"config"`
}

// Interface is a network interface a firewall can face the internet with.
type Interface struct {
	Name   string `json:"name"`
	Uplink bool   `json:"uplink"` // the default route leaves through it
	Client bool   `json:"client"` // this request came in through it
}

// Handler serves the API with relative routes.
func (s *Service) Handler() http.Handler {
	mux := http.NewServeMux()
	handle := func(pattern string, fn func(*http.Request) (any, error)) {
		mux.HandleFunc(pattern, func(w http.ResponseWriter, r *http.Request) {
			if !s.supported {
				writeError(w, http.StatusServiceUnavailable, "unsupported", "a firewall is only managed on Linux", nil)
				return
			}
			data, err := fn(r)
			respond(w, data, err)
		})
	}
	mux.HandleFunc("GET /info", func(w http.ResponseWriter, r *http.Request) {
		respond(w, Info{Supported: s.supported, Available: s.supported && s.engine.Available(), Forwarding: system.ForwardingNow()}, nil)
	})
	handle("GET /config", func(r *http.Request) (any, error) {
		s.mu.Lock()
		defer s.mu.Unlock()
		return ConfigView{Config: s.saved}, nil
	})
	handle("POST /config/preview", s.preview)
	handle("PUT /config", s.apply)
	handle("GET /rules", func(r *http.Request) (any, error) {
		text, err := s.engine.Rules(r.Context())
		return map[string]string{"rules": text}, err
	})
	handle("GET /interfaces", s.interfaces)
	handle("GET /trial", func(*http.Request) (any, error) { return s.status(), nil })
	handle("POST /trial/keep", func(*http.Request) (any, error) { return s.keep() })
	handle("POST /trial/revert", func(*http.Request) (any, error) { return s.revertNow() })
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		writeError(w, http.StatusNotFound, "not_found", "no such endpoint", nil)
	})
	return mux
}

// checked reads a configuration from the request and has nft check it,
// before anything changes.
func (s *Service) checked(r *http.Request) (system.Config, error) {
	if !s.engine.Available() {
		return system.Config{}, &errInUse{"nft is not installed; install the package nftables."}
	}
	var body system.Config
	r.Body = http.MaxBytesReader(nil, r.Body, 1<<20)
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		return system.Config{}, &badRequest{"invalid JSON: " + err.Error()}
	}
	c, err := body.Normalize()
	if err != nil {
		return system.Config{}, err
	}
	if err := s.engine.Check(r.Context(), c); err != nil {
		return system.Config{}, err
	}
	return c, nil
}

// Preview is what a configuration would put in place.
type Preview struct {
	Script string `json:"script"`
}

func (s *Service) preview(r *http.Request) (any, error) {
	c, err := s.checked(r)
	if err != nil {
		return nil, err
	}
	return Preview{Script: c.Script()}, nil
}

func (s *Service) apply(r *http.Request) (any, error) {
	c, err := s.checked(r)
	if err != nil {
		return nil, err
	}
	return s.try(c)
}

func (s *Service) interfaces(r *http.Request) (any, error) {
	out, _, err := s.run.Run(r.Context(), "", "ip", "-j", "link", "show")
	if err != nil {
		return nil, err
	}
	var links []struct {
		Name     string `json:"ifname"`
		LinkType string `json:"link_type"`
	}
	if err := json.Unmarshal(out, &links); err != nil {
		return nil, err
	}
	uplinks := map[string]bool{}
	for _, flag := range []string{"-4", "-6"} {
		out, _, err := s.run.Run(r.Context(), "", "ip", "-j", flag, "route", "show", "default")
		if err != nil {
			continue
		}
		var routes []struct {
			Dev string `json:"dev"`
		}
		if json.Unmarshal(out, &routes) == nil {
			for _, route := range routes {
				uplinks[route.Dev] = true
			}
		}
	}
	client := s.clientDevice(r)
	list := []Interface{}
	for _, l := range links {
		if l.LinkType != "loopback" {
			list = append(list, Interface{Name: l.Name, Uplink: uplinks[l.Name], Client: l.Name == client})
		}
	}
	return list, nil
}

// clientDevice is the interface this request came in through, "" for a
// client on this machine. A reverse proxy on loopback (the panel's own
// gateway) passes the real client along. It only decides which warnings
// show; it is not access control.
func (s *Service) clientDevice(r *http.Request) string {
	ip := remoteIP(r)
	if ip == "" {
		return ""
	}
	out, _, err := s.run.Run(r.Context(), "", "ip", "-j", "route", "get", ip)
	var raw []struct {
		Dev string `json:"dev"`
	}
	if err != nil || json.Unmarshal(out, &raw) != nil || len(raw) == 0 || raw[0].Dev == "lo" {
		return ""
	}
	return raw[0].Dev
}

func remoteIP(r *http.Request) string {
	host := r.RemoteAddr
	if a, err := netip.ParseAddrPort(host); err == nil {
		host = a.Addr().Unmap().WithZone("").String()
	}
	addr, err := netip.ParseAddr(host)
	if err != nil {
		return ""
	}
	if addr.IsLoopback() {
		parts := strings.Split(r.Header.Get("X-Forwarded-For"), ",")
		last := strings.TrimSpace(parts[len(parts)-1])
		if last == "" {
			last = strings.TrimSpace(r.Header.Get("X-Real-IP"))
		}
		if addr, err = netip.ParseAddr(last); err != nil || addr.IsLoopback() {
			return ""
		}
	}
	return addr.WithZone("").String()
}

type badRequest struct{ message string }

func (e *badRequest) Error() string { return e.message }

func respond(w http.ResponseWriter, data any, err error) {
	if err == nil {
		writeJSON(w, http.StatusOK, map[string]any{"data": data})
		return
	}
	var (
		validation *system.ValidationError
		inUse      *errInUse
		bad        *badRequest
		exit       *system.ExitError
	)
	switch {
	case errors.As(err, &validation):
		writeError(w, http.StatusUnprocessableEntity, "invalid", err.Error(), validation.Fields)
	case errors.Is(err, errNoTrial):
		writeError(w, http.StatusConflict, "conflict", err.Error(), nil)
	case errors.As(err, &inUse):
		writeError(w, http.StatusConflict, "in_use", err.Error(), nil)
	case errors.As(err, &bad):
		writeError(w, http.StatusBadRequest, "bad_request", err.Error(), nil)
	case errors.As(err, &exit):
		writeError(w, http.StatusBadGateway, "command_failed", err.Error(), nil)
	default:
		writeError(w, http.StatusInternalServerError, "internal", err.Error(), nil)
	}
}

func writeError(w http.ResponseWriter, status int, code, message string, fields map[string]string) {
	body := map[string]any{"code": code, "message": message}
	if fields != nil {
		body["fields"] = fields
	}
	writeJSON(w, status, map[string]any{"error": body})
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(v)
}

// Package api is the network manager's JSON API: interfaces, routes, DNS
// servers, listening ports and diagnostics to read, and interface settings
// to change. A change that could cut this machine off takes effect on trial:
// it reverts by itself unless it is kept in time, which needs a working
// connection to the panel.
package api

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"net/netip"
	"os"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/lsongdev/ohmyserver/modules/netmgr/system"
)

// DefaultWindow is how long a change waits to be kept. It allows for
// finding the panel again at a new address.
const DefaultWindow = 2 * time.Minute

// applyTimeout bounds commands that change the network. It does not depend
// on the request: the change may well end the client's connection.
const applyTimeout = 45 * time.Second

// connectTimeout bounds joining a Wi-Fi network, which waits up to 30 seconds
// for the network to answer before the commands around it.
const connectTimeout = 75 * time.Second

// Options configures the API.
type Options struct {
	Logger *slog.Logger
	// Runner runs commands; nil runs them on this host.
	Runner system.Runner
	// Backend overrides detecting the network manager, for tests.
	Backend system.Backend
	// Installed overrides looking for iproute2, for tests.
	Installed *bool
	// StateDir keeps the change on trial across restarts, so it still reverts
	// after a crash. Without it, changes are refused.
	StateDir string
	// Window overrides DefaultWindow.
	Window time.Duration
	// ResolvConf is read for DNS servers; empty is /etc/resolv.conf.
	ResolvConf string
	// Probe overrides the connectivity check's network operations, for tests.
	Probe *system.Probe
}

// Service serves the API.
type Service struct {
	run        system.Runner
	backend    system.Backend
	installed  bool
	window     time.Duration
	resolvConf string
	logger     *slog.Logger
	trial      *trials
	probe      system.Probe
}

// New finds the network tools, and finishes a change left on trial by a
// previous run.
func New(opts Options) *Service {
	run := opts.Runner
	if run == nil {
		run = system.Exec{}
	}
	logger := opts.Logger
	if logger == nil {
		logger = slog.Default()
	}
	installed := system.Installed()
	if opts.Installed != nil {
		installed = *opts.Installed
	}
	backend := opts.Backend
	if backend == nil && installed {
		backend = system.Detect(context.Background(), run)
	}
	window := opts.Window
	if window <= 0 {
		window = DefaultWindow
	}
	resolvConf := opts.ResolvConf
	if resolvConf == "" {
		resolvConf = "/etc/resolv.conf"
	}
	probe := system.DefaultProbe(run)
	if opts.Probe != nil {
		probe = *opts.Probe
	}
	s := &Service{run: run, backend: backend, installed: installed, window: window, resolvConf: resolvConf, logger: logger, probe: probe}
	s.trial = newTrials(opts.StateDir, run, opts.Backend, logger)
	return s
}

// Supported reports whether this machine has iproute2. Without it every
// endpoint but /info answers 503 "unsupported", so a shell may leave the
// module out.
func (s *Service) Supported() bool { return s.installed }

// Close stops waiting on a change on trial. Its record stays, so the next
// start picks it up.
func (s *Service) Close() error {
	s.trial.close()
	return nil
}

// Info describes the machine.
type Info struct {
	Supported bool             `json:"supported"`
	Backend   string           `json:"backend,omitempty"` // empty: settings cannot be changed here
	Hostname  string           `json:"hostname,omitempty"`
	Features  *system.Features `json:"features,omitempty"` // Wi-Fi and dial-up, with NetworkManager only
}

// featureBackend is a network manager that can also say what else it does.
type featureBackend interface {
	DetectFeatures(ctx context.Context) system.Features
}

// Interface is a link as the pages show it.
type Interface struct {
	system.Link
	// Reason says why its settings cannot be changed, if so.
	Reason string `json:"reason,omitempty"`
	// Protected: taking it down could cut this machine or this client off.
	Protected bool `json:"protected"`
}

func (s *Service) view(l system.Link, clientDevice string) Interface {
	v := Interface{Link: l, Reason: system.EditableReason(l), Protected: l.Uplink || l.Name == clientDevice}
	return v
}

// clientDevice is the interface this request's client is reached through,
// "" for a client on this machine. A reverse proxy on loopback (the panel's
// own gateway) passes the real client along. This only decides which
// warnings show and which interfaces cannot be taken down; it is not access
// control, so a forged header can only remove a protection from its sender.
func (s *Service) clientDevice(r *http.Request) string {
	ip := remoteIP(r)
	if ip == "" {
		return ""
	}
	device := system.DeviceTo(r.Context(), s.run, ip)
	if device == "lo" {
		return ""
	}
	return device
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
		forwarded := r.Header.Get("X-Forwarded-For")
		parts := strings.Split(forwarded, ",")
		last := strings.TrimSpace(parts[len(parts)-1])
		if last == "" {
			last = strings.TrimSpace(r.Header.Get("X-Real-IP"))
		}
		addr, err = netip.ParseAddr(last)
		if err != nil || addr.IsLoopback() {
			return ""
		}
	}
	return addr.WithZone("").String()
}

// Handler serves the API with relative routes.
func (s *Service) Handler() http.Handler {
	mux := http.NewServeMux()
	handle := func(pattern string, fn func(*http.Request) (any, error)) {
		mux.HandleFunc(pattern, func(w http.ResponseWriter, r *http.Request) {
			if !s.installed {
				writeError(w, http.StatusServiceUnavailable, "unsupported", "iproute2 is not installed on this machine", nil)
				return
			}
			data, err := fn(r)
			respond(w, data, err)
		})
	}
	mux.HandleFunc("GET /info", func(w http.ResponseWriter, r *http.Request) {
		info := Info{Supported: s.installed}
		if s.backend != nil {
			info.Backend = s.backend.Name()
			if fb, ok := s.backend.(featureBackend); ok {
				features := fb.DetectFeatures(r.Context())
				info.Features = &features
			}
		}
		info.Hostname, _ = os.Hostname()
		respond(w, info, nil)
	})
	handle("GET /interfaces", func(r *http.Request) (any, error) {
		links, err := system.Links(r.Context(), s.run)
		if err != nil {
			return nil, err
		}
		device := s.clientDevice(r)
		views := make([]Interface, 0, len(links))
		for _, l := range links {
			views = append(views, s.view(l, device))
		}
		return views, nil
	})
	handle("GET /interfaces/{name}", func(r *http.Request) (any, error) {
		l, err := system.FindLink(r.Context(), s.run, r.PathValue("name"))
		if err != nil {
			return nil, err
		}
		return s.view(*l, s.clientDevice(r)), nil
	})
	handle("POST /interfaces/{name}/up", func(r *http.Request) (any, error) { return s.setUp(r, true) })
	handle("POST /interfaces/{name}/down", func(r *http.Request) (any, error) { return s.setUp(r, false) })
	handle("GET /interfaces/{name}/config", s.readConfig)
	handle("POST /interfaces/{name}/config/preview", s.previewConfig)
	handle("PUT /interfaces/{name}/config", s.applyConfig)
	// A check opens connections to other hosts, so it is a POST: nothing
	// that merely fetches a page should cause it.
	handle("POST /connectivity/check", func(r *http.Request) (any, error) {
		result := s.probe.Check(r.Context())
		s.logger.Info("connectivity checked", "result", result.String())
		return result, nil
	})
	s.registerBridges(handle)
	s.registerWifi(handle)
	s.registerPPPoE(handle)
	s.registerWifiAP(handle)
	handle("GET /trial", func(r *http.Request) (any, error) { return s.trial.status(), nil })
	handle("POST /trial/keep", func(r *http.Request) (any, error) { return s.trial.keep() })
	handle("POST /trial/revert", func(r *http.Request) (any, error) { return s.trial.revertNow() })
	handle("GET /routes", func(r *http.Request) (any, error) {
		routes, err := system.Routes(r.Context(), s.run)
		if routes == nil {
			routes = []system.Route{}
		}
		return routes, err
	})
	handle("GET /resolvers", func(r *http.Request) (any, error) {
		return system.ReadResolvers(r.Context(), s.run, s.resolvConf), nil
	})
	handle("GET /ports", func(r *http.Request) (any, error) { return system.Ports(r.Context(), s.run) })
	handle("POST /ports/{pid}/kill", func(r *http.Request) (any, error) { return s.killPort(r) })
	mux.HandleFunc("POST /tools/run", s.runTool)
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		writeError(w, http.StatusNotFound, "not_found", "no such endpoint", nil)
	})
	return mux
}

// killPort sends SIGTERM only to a process that still owns a listening
// socket. Rechecking the PID avoids acting on stale rows or arbitrary PIDs.
func (s *Service) killPort(r *http.Request) (any, error) {
	pid, err := strconv.Atoi(r.PathValue("pid"))
	if err != nil || pid <= 1 || pid == os.Getpid() {
		return nil, &badRequest{"invalid process id"}
	}
	ports, err := system.Ports(r.Context(), s.run)
	if err != nil {
		return nil, err
	}
	for _, port := range ports {
		if port.PID == pid {
			if err := syscall.Kill(pid, syscall.SIGTERM); err != nil {
				return nil, err
			}
			return map[string]int{"pid": pid}, nil
		}
	}
	return nil, &badRequest{"process is no longer listening on a port"}
}

// errInUse refuses what would cut this machine or this client off.
type errInUse struct{ message string }

func (e *errInUse) Error() string { return e.message }

func (s *Service) setUp(r *http.Request, up bool) (any, error) {
	name := r.PathValue("name")
	l, err := system.FindLink(r.Context(), s.run, name)
	if err != nil {
		return nil, err
	}
	view := s.view(*l, s.clientDevice(r))
	if view.Reason != "" {
		return nil, &errInUse{view.Reason}
	}
	if s.trial.status().Trial != nil {
		return nil, &errInUse{"A change is on trial; keep or revert it first."}
	}
	if !up && view.Protected {
		return nil, &errInUse{name + " carries this machine's default route or this connection; taking it down could leave the panel unreachable."}
	}
	ctx, cancel := context.WithTimeout(context.Background(), applyTimeout)
	defer cancel()
	if s.backend != nil {
		err = s.backend.SetUp(ctx, name, up)
	} else {
		state := "down"
		if up {
			state = "up"
		}
		_, _, err = s.run.Run(ctx, "", "ip", "link", "set", "dev", name, state)
	}
	if err != nil {
		return nil, err
	}
	s.logger.Info("interface state changed", "interface", name, "up", up)
	return map[string]bool{"up": up}, nil
}

// Config is the settings page's data.
type Config struct {
	*system.Setting
	Backend   string `json:"backend"`
	Protected bool   `json:"protected"`
}

func (s *Service) readConfig(r *http.Request) (any, error) {
	l, err := system.FindLink(r.Context(), s.run, r.PathValue("name"))
	if err != nil {
		return nil, err
	}
	view := s.view(*l, s.clientDevice(r))
	if s.backend == nil {
		return Config{
			Setting:   &system.Setting{Config: system.Config{Addresses: []string{}, DNS: []string{}, Routes: []system.StaticRoute{}}, Reason: "Neither NetworkManager nor systemd-networkd runs on this machine, so settings cannot be changed here."},
			Protected: view.Protected,
		}, nil
	}
	setting, err := s.backend.Read(r.Context(), *l)
	if err != nil {
		return nil, err
	}
	if view.Reason != "" {
		setting.Reason = view.Reason
	}
	return Config{Setting: setting, Backend: s.backend.Name(), Protected: view.Protected}, nil
}

type configBody struct {
	Version string        `json:"version"`
	Config  system.Config `json:"config"`
}

func (s *Service) changeable(r *http.Request) (*system.Link, *configBody, error) {
	if s.backend == nil {
		return nil, nil, &errInUse{"No network manager runs on this machine."}
	}
	l, err := system.FindLink(r.Context(), s.run, r.PathValue("name"))
	if err != nil {
		return nil, nil, err
	}
	var body configBody
	if err := decode(r, &body); err != nil {
		return nil, nil, err
	}
	if body.Config, err = body.Config.Normalize(); err != nil {
		return nil, nil, err
	}
	if reason := system.EditableReason(*l); reason != "" {
		return nil, nil, &errInUse{reason}
	}
	if body.Config.Metric != 0 && !system.SupportsMetric(s.backend) {
		return nil, nil, &system.ValidationError{Fields: map[string]string{"metric": s.backend.Name() + " cannot set a route priority here"}}
	}
	return l, &body, nil
}

func (s *Service) previewConfig(r *http.Request) (any, error) {
	l, body, err := s.changeable(r)
	if err != nil {
		return nil, err
	}
	return s.backend.Plan(r.Context(), *l, body.Config)
}

func (s *Service) applyConfig(r *http.Request) (any, error) {
	l, body, err := s.changeable(r)
	if err != nil {
		return nil, err
	}
	// Commands continue if the change ends this client's connection.
	ctx, cancel := context.WithTimeout(context.Background(), applyTimeout)
	defer cancel()
	backend, link, cfg := s.backend, *l, body.Config
	return s.trial.start(ctx, change{
		subject: link.Name,
		begin: func(ctx context.Context) (string, system.Backend, system.Undo, error) {
			current, err := backend.Read(ctx, link)
			if err != nil {
				return "", nil, system.Undo{}, err
			}
			if current.Reason != "" {
				return "", nil, system.Undo{}, &errInUse{current.Reason}
			}
			if body.Version != current.Version {
				return "", nil, system.Undo{}, system.ErrConflict
			}
			undo, err := backend.Prepare(ctx, link)
			return current.Target, backend, undo, err
		},
		apply: func(ctx context.Context, b system.Backend) error { return b.Apply(ctx, link, cfg) },
	}, s.window)
}

func decode(r *http.Request, v any) error {
	r.Body = http.MaxBytesReader(nil, r.Body, 1<<20)
	if err := json.NewDecoder(r.Body).Decode(v); err != nil {
		return &badRequest{"invalid JSON: " + err.Error()}
	}
	return nil
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
	case errors.Is(err, system.ErrNotFound):
		writeError(w, http.StatusNotFound, "not_found", err.Error(), nil)
	case errors.Is(err, system.ErrConflict), errors.Is(err, errNoTrial):
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

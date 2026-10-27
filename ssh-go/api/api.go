// Package api manages saved SSH hosts and opens terminals on them, as a
// JSON API with relative routes plus a WebSocket terminal.
package api

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"strconv"
	"strings"
	"sync/atomic"
	"time"

	"golang.org/x/crypto/ssh"

	"github.com/lsongdev/ssh-go/hosts"
	"github.com/lsongdev/ssh-go/store"
)

// Input creates or updates a host. On update, empty secrets keep the
// stored ones.
type Input struct {
	Name        string `json:"name"`
	Address     string `json:"address"`
	Port        int    `json:"port"`
	User        string `json:"user"`
	Auth        string `json:"auth"`
	Password    string `json:"password"`
	PrivateKey  string `json:"private_key"`
	Passphrase  string `json:"passphrase"`
	Description string `json:"description"`
}

// apply copies in onto h, keeping h's secrets where in leaves them empty.
func (in Input) apply(h *hosts.Host) {
	h.Name, h.Address, h.Port, h.User, h.Auth, h.Description = in.Name, in.Address, in.Port, in.User, in.Auth, in.Description
	if in.Password != "" {
		h.Password = in.Password
	}
	if in.PrivateKey != "" {
		h.PrivateKey, h.Passphrase = in.PrivateKey, in.Passphrase
	} else if in.Passphrase != "" {
		h.Passphrase = in.Passphrase
	}
	// Secrets of other methods are not kept around.
	if h.Auth != hosts.AuthPassword {
		h.Password = ""
	}
	if h.Auth != hosts.AuthKey {
		h.PrivateKey, h.Passphrase = "", ""
	}
}

// TestResult reports a successful test connection.
type TestResult struct {
	HostKeyType   string `json:"host_key_type"`
	HostKeySHA256 string `json:"host_key_sha256"`
	Pinned        bool   `json:"pinned"` // the key was already pinned
}

// Summary counts hosts and open terminals for an overview.
type Summary struct {
	Hosts    int   `json:"hosts"`
	Sessions int64 `json:"sessions"`
}

// panelKeySetting stores the panel's private key in ssh_settings.
const panelKeySetting = "panel_key"

// Service manages hosts.
type Service struct {
	store     *store.SQLite
	log       *slog.Logger
	key       ssh.Signer
	publicKey string // authorized_keys line of key
	sessions  atomic.Int64
}

// NewService returns the service, generating the panel key on first use.
func NewService(ctx context.Context, st *store.SQLite, log *slog.Logger) (*Service, error) {
	if log == nil {
		log = slog.Default()
	}
	s := &Service{store: st, log: log}
	private, err := st.Setting(ctx, panelKeySetting)
	if err != nil {
		return nil, err
	}
	if private == "" {
		if private, err = hosts.NewKey("ssh-go"); err != nil {
			return nil, err
		}
		if err := st.SetSetting(ctx, panelKeySetting, private); err != nil {
			return nil, err
		}
	}
	if s.publicKey, s.key, err = hosts.PublicKey(private, "ssh-go"); err != nil {
		return nil, fmt.Errorf("panel key: %w", err)
	}
	return s, nil
}

// PublicKey returns the panel's public key in authorized_keys format: add
// it to ~/.ssh/authorized_keys on hosts that use the "panel" method.
func (s *Service) PublicKey() string { return s.publicKey }

// List returns every host.
func (s *Service) List(ctx context.Context) ([]hosts.View, error) {
	list, err := s.store.List(ctx)
	out := make([]hosts.View, len(list))
	for i, h := range list {
		out[i] = h.View()
	}
	return out, err
}

// Get returns one host.
func (s *Service) Get(ctx context.Context, id int64) (hosts.View, error) {
	h, err := s.store.Get(ctx, id)
	return h.View(), err
}

// Create saves a new host.
func (s *Service) Create(ctx context.Context, in Input) (hosts.View, error) {
	var h hosts.Host
	in.apply(&h)
	if err := h.Normalize(); err != nil {
		return hosts.View{}, err
	}
	err := s.store.Create(ctx, &h)
	return h.View(), err
}

// Update changes a host. A new address or port forgets the pinned host key,
// since it belongs to the old server.
func (s *Service) Update(ctx context.Context, id int64, in Input) (hosts.View, error) {
	h, err := s.store.Get(ctx, id)
	if err != nil {
		return hosts.View{}, err
	}
	address, port := h.Address, h.Port
	in.apply(&h)
	if err := h.Normalize(); err != nil {
		return hosts.View{}, err
	}
	if h.Address != address || h.Port != port {
		h.HostKey = ""
	}
	err = s.store.Update(ctx, &h)
	return h.View(), err
}

// Delete removes a host.
func (s *Service) Delete(ctx context.Context, id int64) error { return s.store.Delete(ctx, id) }

// ForgetHostKey clears the pinned key, e.g. after a server was reinstalled.
func (s *Service) ForgetHostKey(ctx context.Context, id int64) error {
	return s.store.ForgetHostKey(ctx, id)
}

// Summary counts hosts and open terminals.
func (s *Service) Summary(ctx context.Context) (Summary, error) {
	list, err := s.store.List(ctx)
	return Summary{Hosts: len(list), Sessions: s.sessions.Load()}, err
}

// Test connects with in, merged into the stored host id when id > 0 (its
// secrets and pinned key), without saving anything.
func (s *Service) Test(ctx context.Context, id int64, in Input) (TestResult, error) {
	var h hosts.Host
	if id > 0 {
		var err error
		if h, err = s.store.Get(ctx, id); err != nil {
			return TestResult{}, err
		}
		if h.Address != strings.TrimSpace(in.Address) || h.Port != in.Port {
			h.HostKey = ""
		}
	}
	in.apply(&h)
	if err := h.Normalize(); err != nil {
		return TestResult{}, err
	}
	conn, err := hosts.Dial(ctx, h, s.key)
	if err != nil {
		return TestResult{}, &ConnectError{err}
	}
	defer conn.Close()
	key, _, _, _, _ := ssh.ParseAuthorizedKey([]byte(conn.HostKey))
	return TestResult{HostKeyType: key.Type(), HostKeySHA256: ssh.FingerprintSHA256(key), Pinned: h.HostKey != ""}, nil
}

// Connect opens an authenticated connection to a stored host, pinning its
// key on first use.
func (s *Service) Connect(ctx context.Context, id int64) (*hosts.Conn, error) {
	h, err := s.store.Get(ctx, id)
	if err != nil {
		return nil, err
	}
	conn, err := hosts.Dial(ctx, h, s.key)
	if err != nil {
		return nil, &ConnectError{err}
	}
	if err := s.store.Connected(context.WithoutCancel(ctx), id, conn.HostKey); err != nil {
		s.log.Warn("ssh: recording the connection failed", "host", h.Name, "err", err)
	}
	return conn, nil
}

// ConnectError wraps a failure to reach or authenticate to a host.
type ConnectError struct{ Err error }

func (e *ConnectError) Error() string { return "connect: " + e.Err.Error() }
func (e *ConnectError) Unwrap() error { return e.Err }

// Handler serves the API. Successful responses are {"data": ...}; failures
// are {"error": {"code": ..., "message": ...}} with a matching status.
func (s *Service) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /hosts", func(w http.ResponseWriter, r *http.Request) {
		list, err := s.List(r.Context())
		respond(w, http.StatusOK, list, err)
	})
	mux.HandleFunc("POST /hosts", func(w http.ResponseWriter, r *http.Request) {
		var in Input
		if decode(w, r, &in) {
			h, err := s.Create(r.Context(), in)
			respond(w, http.StatusCreated, h, err)
		}
	})
	mux.HandleFunc("POST /test", func(w http.ResponseWriter, r *http.Request) {
		var in struct {
			ID int64 `json:"id"`
			Input
		}
		if decode(w, r, &in) {
			res, err := s.Test(r.Context(), in.ID, in.Input)
			respond(w, http.StatusOK, res, err)
		}
	})
	mux.HandleFunc("GET /hosts/{id}", func(w http.ResponseWriter, r *http.Request) {
		if id, ok := pathID(w, r); ok {
			h, err := s.Get(r.Context(), id)
			respond(w, http.StatusOK, h, err)
		}
	})
	mux.HandleFunc("PUT /hosts/{id}", func(w http.ResponseWriter, r *http.Request) {
		var in Input
		if id, ok := pathID(w, r); ok && decode(w, r, &in) {
			h, err := s.Update(r.Context(), id, in)
			respond(w, http.StatusOK, h, err)
		}
	})
	mux.HandleFunc("DELETE /hosts/{id}", func(w http.ResponseWriter, r *http.Request) {
		if id, ok := pathID(w, r); ok {
			respond(w, http.StatusNoContent, nil, s.Delete(r.Context(), id))
		}
	})
	mux.HandleFunc("DELETE /hosts/{id}/host-key", func(w http.ResponseWriter, r *http.Request) {
		if id, ok := pathID(w, r); ok {
			respond(w, http.StatusNoContent, nil, s.ForgetHostKey(r.Context(), id))
		}
	})
	mux.HandleFunc("GET /hosts/{id}/terminal", s.terminal)
	mux.HandleFunc("GET /key", func(w http.ResponseWriter, r *http.Request) {
		respond(w, http.StatusOK, map[string]string{"public_key": s.publicKey}, nil)
	})
	mux.HandleFunc("GET /summary", func(w http.ResponseWriter, r *http.Request) {
		sum, err := s.Summary(r.Context())
		respond(w, http.StatusOK, sum, err)
	})
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		writeError(w, http.StatusNotFound, "not_found", "no such API endpoint")
	})
	return mux
}

func respond(w http.ResponseWriter, status int, data any, err error) {
	var hostKey *hosts.HostKeyError
	var connect *ConnectError
	switch {
	case err == nil:
	case errors.Is(err, hosts.ErrInvalid):
		writeError(w, http.StatusUnprocessableEntity, "invalid_request", err.Error())
		return
	case errors.Is(err, store.ErrNotFound):
		writeError(w, http.StatusNotFound, "not_found", err.Error())
		return
	case errors.Is(err, store.ErrConflict):
		writeError(w, http.StatusConflict, "conflict", err.Error())
		return
	case errors.As(err, &hostKey):
		writeError(w, http.StatusConflict, "host_key_changed", hostKey.Error())
		return
	case errors.As(err, &connect):
		writeError(w, http.StatusBadGateway, "connect_failed", connect.Err.Error())
		return
	default:
		writeError(w, http.StatusInternalServerError, "internal", err.Error())
		return
	}
	if status == http.StatusNoContent || data == nil {
		w.WriteHeader(status)
		return
	}
	writeJSON(w, status, map[string]any{"data": data})
}

func writeError(w http.ResponseWriter, status int, code, message string) {
	writeJSON(w, status, map[string]any{"error": map[string]string{"code": code, "message": message}})
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(v)
}

func decode(w http.ResponseWriter, r *http.Request, v any) bool {
	dec := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<20))
	dec.DisallowUnknownFields()
	if err := dec.Decode(v); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_request", "invalid JSON body: "+err.Error())
		return false
	}
	return true
}

func pathID(w http.ResponseWriter, r *http.Request) (int64, bool) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil || id < 1 {
		writeError(w, http.StatusBadRequest, "invalid_request", "invalid id")
		return 0, false
	}
	return id, true
}

// keepAliveInterval is how often terminals check that the server is alive.
var keepAliveInterval = 30 * time.Second

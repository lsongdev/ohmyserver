// Package web exposes the embedded Xray engine as a panel module.
package web

import (
	"embed"
	"encoding/json"
	"errors"
	"io"
	"io/fs"
	"net/http"

	"github.com/lsongdev/ohmyserver/modules/xray/engine"
)

//go:embed ui
var embedded embed.FS

type Module struct{ engine *engine.Engine }

func New(dir string) (*Module, error) {
	e, err := engine.Open(dir)
	if err != nil {
		return nil, err
	}
	return &Module{engine: e}, nil
}

func (m *Module) Close() error { return m.engine.Close() }

func (m *Module) Assets() fs.FS {
	ui, _ := fs.Sub(embedded, "ui")
	return ui
}

type request struct {
	Revision *uint64         `json:"revision"`
	Config   json.RawMessage `json:"config"`
}

func (m *Module) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /config", func(w http.ResponseWriter, r *http.Request) {
		reply(w, http.StatusOK, map[string]any{"data": m.engine.Snapshot()})
	})
	mux.HandleFunc("PUT /config", mutate(func(req request) (engine.Snapshot, error) {
		return m.engine.Save(*req.Revision, req.Config)
	}))
	mux.HandleFunc("POST /apply", mutate(func(req request) (engine.Snapshot, error) {
		return m.engine.Apply(*req.Revision)
	}))
	mux.HandleFunc("POST /stop", mutate(func(req request) (engine.Snapshot, error) {
		return m.engine.Stop(*req.Revision)
	}))
	return mux
}

func mutate(fn func(request) (engine.Snapshot, error)) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req request
		d := json.NewDecoder(http.MaxBytesReader(w, r.Body, 2<<20))
		d.DisallowUnknownFields()
		if err := d.Decode(&req); err != nil {
			fail(w, http.StatusBadRequest, err.Error())
			return
		}
		if req.Revision == nil || d.Decode(new(any)) != io.EOF {
			fail(w, http.StatusBadRequest, "expected one JSON object with a revision")
			return
		}
		data, err := fn(req)
		switch {
		case err == nil:
			reply(w, http.StatusOK, map[string]any{"data": data})
		case errors.Is(err, engine.ErrConflict):
			fail(w, http.StatusConflict, err.Error())
		case errors.Is(err, engine.ErrInvalid):
			fail(w, http.StatusUnprocessableEntity, err.Error())
		case errors.Is(err, engine.ErrClosed):
			fail(w, http.StatusServiceUnavailable, err.Error())
		default:
			fail(w, http.StatusInternalServerError, err.Error())
		}
	}
}

func fail(w http.ResponseWriter, status int, message string) {
	reply(w, status, map[string]string{"error": message})
}

func reply(w http.ResponseWriter, status int, payload any) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(payload)
}

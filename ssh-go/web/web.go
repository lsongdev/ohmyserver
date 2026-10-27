// Package web assembles the SSH terminal as a panel module: saved hosts in
// SQLite, the JSON API with its WebSocket terminal, and the browser UI
// (native ES modules in ui/, no build step).
//
// An SSH value satisfies the panel module contract without depending on
// any panel: Handler serves the API with relative routes and Assets holds
// the UI with manifest.json at its root. The standalone command and a panel
// build it the same way, through New.
package web

import (
	"context"
	"database/sql"
	"embed"
	"io/fs"
	"log/slog"
	"net/http"

	"github.com/lsongdev/ohmyserver/ssh-go/api"
	"github.com/lsongdev/ohmyserver/ssh-go/store"
)

//go:embed ui
var embedded embed.FS

// UI is the module's browser assets; manifest.json describes them.
var UI = func() fs.FS {
	sub, err := fs.Sub(embedded, "ui")
	if err != nil {
		panic(err) // the embed directive guarantees the directory exists
	}
	return sub
}()

// Options configures the module.
type Options struct {
	Logger *slog.Logger
}

// SSH is the assembled module.
type SSH struct {
	Service *api.Service
	store   *store.SQLite
}

// New migrates the ssh_ tables in db (shareable with other modules). The
// caller owns db.
func New(ctx context.Context, db *sql.DB, opts Options) (*SSH, error) {
	st, err := store.New(ctx, db)
	if err != nil {
		return nil, err
	}
	return NewWithStore(ctx, st, opts)
}

// NewWithStore assembles the module on st, which it closes on Close.
func NewWithStore(ctx context.Context, st *store.SQLite, opts Options) (*SSH, error) {
	service, err := api.NewService(ctx, st, opts.Logger)
	if err != nil {
		return nil, err
	}
	return &SSH{Service: service, store: st}, nil
}

// Handler serves the module's API with relative routes.
func (s *SSH) Handler() http.Handler { return s.Service.Handler() }

// Assets returns the browser UI; manifest.json is at its root.
func (s *SSH) Assets() fs.FS { return UI }

// Close releases the store. Open terminals end with their HTTP server; a
// database passed to New stays open.
func (s *SSH) Close() error { return s.store.Close() }

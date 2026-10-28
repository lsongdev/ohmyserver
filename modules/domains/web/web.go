// Package web assembles the domains module: DNS provider accounts in
// SQLite, the JSON API, and the browser UI (native ES modules in ui/, no
// build step).
//
// A Domains value satisfies the panel module contract without depending
// on any panel: Handler serves the API with relative routes and Assets
// holds the UI with manifest.json at its root. Challenges publishes ACME
// DNS-01 records in the accounts' zones, for a panel to hand to its
// certificate manager.
package web

import (
	"context"
	"database/sql"
	"embed"
	"io/fs"
	"log/slog"
	"net/http"

	"github.com/lsongdev/ohmyserver/modules/domains/api"
	"github.com/lsongdev/ohmyserver/modules/domains/store"
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

// Domains is the assembled module.
type Domains struct {
	Service *api.Service
	store   *store.SQLite
}

// New migrates the domains_ tables in db (shareable with other modules).
// The caller owns db.
func New(ctx context.Context, db *sql.DB, opts Options) (*Domains, error) {
	st, err := store.New(ctx, db)
	if err != nil {
		return nil, err
	}
	return NewWithStore(st, opts), nil
}

// NewWithStore assembles the module on st, which it closes on Close.
func NewWithStore(st *store.SQLite, opts Options) *Domains {
	return &Domains{Service: api.NewService(st, opts.Logger), store: st}
}

// Challenges publishes ACME DNS-01 records in the accounts' zones.
func (d *Domains) Challenges() api.Challenges { return api.Challenges{Service: d.Service} }

// Handler serves the module's API with relative routes.
func (d *Domains) Handler() http.Handler { return d.Service.Handler() }

// Assets returns the browser UI; manifest.json is at its root.
func (d *Domains) Assets() fs.FS { return UI }

// Close releases the store; a database passed to New stays open.
func (d *Domains) Close() error { return d.store.Close() }

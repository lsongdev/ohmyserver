// Package web assembles the notification center as a panel module: events and
// issues in SQLite, the JSON API and the browser UI (native ES modules in ui/,
// no build step).
//
// A Notify value satisfies the panel module contract without depending on any
// panel: Handler serves the API with relative routes and Assets holds the UI
// with manifest.json at its root.
package web

import (
	"context"
	"database/sql"
	"embed"
	"io/fs"
	"net/http"

	"github.com/lsongdev/ohmyserver/modules/notify/api"
	"github.com/lsongdev/ohmyserver/modules/notify/store"
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

// Options configures the module; see api.Options.
type Options = api.Options

// Notify is the assembled module.
type Notify struct {
	Service *api.Service
	store   *store.SQLite
}

// New migrates the notify_ tables in db (shareable with other modules) and
// starts the upkeep. The caller owns db.
func New(ctx context.Context, db *sql.DB, opts Options) (*Notify, error) {
	st, err := store.New(ctx, db)
	if err != nil {
		return nil, err
	}
	return NewWithStore(st, opts), nil
}

// NewWithStore assembles the module on st, which it closes on Close.
func NewWithStore(st *store.SQLite, opts Options) *Notify {
	service := api.NewService(st, opts)
	service.Start()
	return &Notify{Service: service, store: st}
}

// Handler serves the module's API with relative routes.
func (n *Notify) Handler() http.Handler { return n.Service.Handler() }

// Assets returns the browser UI; manifest.json is at its root.
func (n *Notify) Assets() fs.FS { return UI }

// Close stops the upkeep. A database passed to New stays open.
func (n *Notify) Close() error {
	n.Service.Close()
	return n.store.Close()
}

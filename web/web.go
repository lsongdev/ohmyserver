// Package web assembles the host monitor as a panel module: the sampler,
// its SQLite history, the JSON API and the browser UI (native ES modules
// in ui/, no build step).
//
// A Monitor value satisfies the panel module contract without depending
// on any panel: Handler serves the API with relative routes and Assets
// holds the UI with manifest.json at its root. The standalone command and
// a panel build it the same way, through New.
package web

import (
	"context"
	"database/sql"
	"embed"
	"io/fs"
	"net/http"

	"github.com/lsongdev/monitor-go/api"
	"github.com/lsongdev/monitor-go/store"
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

// Options configures the monitor; see api.Options. Collectors for other
// kinds of hosts (e.g. SSH hosts) are added here.
type Options = api.Options

// Monitor is the assembled host monitor.
type Monitor struct {
	Service *api.Service
	store   *store.SQLite
}

// New migrates the monitor_ tables in db (shareable with other modules)
// and starts sampling. The caller owns db.
func New(ctx context.Context, db *sql.DB, opts Options) (*Monitor, error) {
	st, err := store.New(ctx, db)
	if err != nil {
		return nil, err
	}
	return NewWithStore(st, opts), nil
}

// NewWithStore assembles the monitor on st, which it closes on Close, and
// starts sampling.
func NewWithStore(st *store.SQLite, opts Options) *Monitor {
	service := api.NewService(st, opts)
	service.Start()
	return &Monitor{Service: service, store: st}
}

// Handler serves the module's API with relative routes.
func (m *Monitor) Handler() http.Handler { return m.Service.Handler() }

// Assets returns the browser UI; manifest.json is at its root.
func (m *Monitor) Assets() fs.FS { return UI }

// Close stops sampling. A database passed to New stays open.
func (m *Monitor) Close() error {
	m.Service.Close()
	return m.store.Close()
}

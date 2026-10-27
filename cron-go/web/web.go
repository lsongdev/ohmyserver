// Package web assembles the job scheduler as a panel module: jobs and their
// history in SQLite, the scheduler, the JSON API and the browser UI (native
// ES modules in ui/, no build step).
//
// A Cron value satisfies the panel module contract without depending on
// any panel: Handler serves the API with relative routes and Assets holds
// the UI with manifest.json at its root. The standalone command and a panel
// build it the same way, through New.
package web

import (
	"context"
	"database/sql"
	"embed"
	"io/fs"
	"net/http"

	"github.com/lsongdev/cron-go/api"
	"github.com/lsongdev/cron-go/store"
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

// Options configures the scheduler; see api.Options. Runners for other
// kinds of targets (e.g. SSH hosts) are added here.
type Options = api.Options

// Cron is the assembled scheduler.
type Cron struct {
	Service *api.Service
	store   *store.SQLite
}

// New migrates the cron_ tables in db (shareable with other modules) and
// schedules the enabled jobs. The caller owns db.
func New(ctx context.Context, db *sql.DB, opts Options) (*Cron, error) {
	st, err := store.New(ctx, db)
	if err != nil {
		return nil, err
	}
	return NewWithStore(ctx, st, opts)
}

// NewWithStore assembles the scheduler on st, which it closes on Close.
func NewWithStore(ctx context.Context, st *store.SQLite, opts Options) (*Cron, error) {
	service := api.NewService(st, opts)
	if err := service.Start(ctx); err != nil {
		return nil, err
	}
	return &Cron{Service: service, store: st}, nil
}

// Handler serves the module's API with relative routes.
func (c *Cron) Handler() http.Handler { return c.Service.Handler() }

// Assets returns the browser UI; manifest.json is at its root.
func (c *Cron) Assets() fs.FS { return UI }

// Close stops scheduling and waits for running jobs, which are recorded as
// interrupted. A database passed to New stays open.
func (c *Cron) Close() error {
	c.Service.Close()
	return c.store.Close()
}

// Package web assembles the CI as a panel module: repositories and builds
// in SQLite, the build queue running steps in Docker containers, the JSON
// API with its webhook endpoint, and the browser UI (native ES modules in
// ui/, no build step).
//
// A CI value satisfies the panel module contract without depending on any
// panel: Handler serves the API with relative routes, Assets holds the UI
// with manifest.json at its root, and the manifest lists the webhook path
// as public. The standalone command and a panel build it the same way.
package web

import (
	"context"
	"database/sql"
	"embed"
	"io/fs"
	"net/http"

	"github.com/lsongdev/ohmyserver/modules/ci/api"
	"github.com/lsongdev/ohmyserver/modules/ci/docker"
	"github.com/lsongdev/ohmyserver/modules/ci/pipeline"
	"github.com/lsongdev/ohmyserver/modules/ci/store"
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

// Options configures the CI; see api.Options. Runtime runs the build
// containers; nil connects to the Docker daemon the docker CLI would use.
type Options struct {
	api.Options
	Runtime pipeline.Runtime
}

// CI is the assembled CI.
type CI struct {
	Service *api.Service
	store   *store.SQLite
}

// New migrates the ci_ tables in db (shareable with other modules) and
// starts the build queue. The caller owns db.
func New(ctx context.Context, db *sql.DB, opts Options) (*CI, error) {
	st, err := store.New(ctx, db)
	if err != nil {
		return nil, err
	}
	return NewWithStore(ctx, st, opts)
}

// NewWithStore assembles the CI on st, which it closes on Close.
func NewWithStore(ctx context.Context, st *store.SQLite, opts Options) (*CI, error) {
	rt := opts.Runtime
	if rt == nil {
		client, err := docker.FromEnv()
		if err != nil {
			return nil, err
		}
		rt = client
	}
	service := api.NewService(st, rt, opts.Options)
	if err := service.Start(ctx); err != nil {
		return nil, err
	}
	return &CI{Service: service, store: st}, nil
}

// Handler serves the module's API with relative routes.
func (c *CI) Handler() http.Handler { return c.Service.Handler() }

// Assets returns the browser UI; manifest.json is at its root.
func (c *CI) Assets() fs.FS { return UI }

// Close cancels running builds (their containers are removed) and waits
// for them. A database passed to New stays open.
func (c *CI) Close() error {
	c.Service.Close()
	return c.store.Close()
}

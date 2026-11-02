// Package web assembles the ZFS snapshot manager as a panel module: the JSON
// API, the scheduler that applies retention policies, and the browser UI
// (native ES modules in ui/, no build step).
//
// A ZFS value satisfies the panel module contract without depending on any
// panel: Handler serves the API with relative routes and Assets holds the UI
// with manifest.json at its root. It keeps no state of its own: snapshots
// and policies live in ZFS.
package web

import (
	"embed"
	"io/fs"
	"net/http"

	"github.com/lsongdev/ohmyserver/modules/zfsmgr/api"
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

// ZFS is the assembled module.
type ZFS struct {
	API *api.Service
}

// New looks for ZFS and, when it is there, starts applying the datasets'
// retention policies. Without it the module still serves, and its pages say
// so; a shell may leave it out.
func New(opts Options) *ZFS {
	service := api.New(opts)
	service.Start()
	return &ZFS{API: service}
}

// Handler serves the module's API with relative routes.
func (z *ZFS) Handler() http.Handler { return z.API.Handler() }

// Assets returns the browser UI; manifest.json is at its root.
func (z *ZFS) Assets() fs.FS { return UI }

// Close stops applying retention policies.
func (z *ZFS) Close() error { return z.API.Close() }

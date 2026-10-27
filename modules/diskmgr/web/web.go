// Package web assembles the disk module for a panel: the JSON API
// and the browser UI (native ES modules in ui/, no build step).
//
// A Disks value satisfies the panel module contract without depending
// on any panel: Handler serves the API with relative routes and Assets
// holds the UI with manifest.json at its root. The standalone command and a
// panel build it the same way, through New. It keeps no state: the kernel and
// smartctl are the source of truth.
package web

import (
	"embed"
	"io/fs"
	"net/http"

	"github.com/lsongdev/ohmyserver/modules/diskmgr/api"
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

// Disks is the assembled module.
type Disks struct {
	API *api.Service
}

// New returns the module. Without lsblk (not Linux) or smartctl it still
// serves, and its pages say what is missing.
func New(opts Options) *Disks {
	return &Disks{API: api.New(opts)}
}

// Handler serves the module's API with relative routes.
func (d *Disks) Handler() http.Handler { return d.API.Handler() }

// Assets returns the browser UI; manifest.json is at its root.
func (d *Disks) Assets() fs.FS { return UI }

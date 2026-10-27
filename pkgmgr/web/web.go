// Package web assembles the package manager as a panel module: the JSON API
// and the browser UI (native ES modules in ui/, no build step).
//
// A Packages value satisfies the panel module contract without depending
// on any panel: Handler serves the API with relative routes and Assets
// holds the UI with manifest.json at its root. The standalone command and a
// panel build it the same way, through New. It keeps no state on disk: the
// package manager is the source of truth, and jobs live in memory.
package web

import (
	"embed"
	"io/fs"
	"net/http"

	"github.com/lsongdev/ohmyserver/pkgmgr/api"
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

// Packages is the assembled module.
type Packages struct {
	Service *api.Service
}

// New detects the system's package manager. Without one the module still
// serves, and its pages say so.
func New(opts Options) *Packages {
	return &Packages{Service: api.NewService(opts)}
}

// Handler serves the module's API with relative routes.
func (p *Packages) Handler() http.Handler { return p.Service.Handler() }

// Assets returns the browser UI; manifest.json is at its root.
func (p *Packages) Assets() fs.FS { return UI }

// Close waits for a running job: a package transaction is never cut short.
func (p *Packages) Close() error {
	p.Service.Wait()
	return nil
}

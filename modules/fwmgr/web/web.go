// Package web assembles the firewall manager as a panel module: the JSON API
// and the browser UI (native ES modules in ui/, no build step).
//
// A Firewall value satisfies the panel module contract without depending on
// any panel: Handler serves the API with relative routes and Assets holds
// the UI with manifest.json at its root. The standalone command and a panel
// build it the same way, through New.
package web

import (
	"embed"
	"io/fs"
	"net/http"

	"github.com/lsongdev/ohmyserver/modules/fwmgr/api"
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

// Firewall is the assembled module.
type Firewall struct {
	API *api.Service
}

// New loads the saved firewall and puts it in effect.
func New(opts Options) *Firewall { return &Firewall{API: api.New(opts)} }

// Handler serves the module's API with relative routes.
func (f *Firewall) Handler() http.Handler { return f.API.Handler() }

// Assets returns the browser UI; manifest.json is at its root.
func (f *Firewall) Assets() fs.FS { return UI }

// Close stops waiting on a change on trial; the next start picks it up.
func (f *Firewall) Close() error { return f.API.Close() }

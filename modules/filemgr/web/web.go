// Package web assembles the file manager for a panel: the JSON API and the
// browser UI (native ES modules in ui/, no build step).
//
// A Files value satisfies the panel module contract without depending on
// any panel: Handler serves the API with relative routes and Assets holds
// the UI with manifest.json at its root. The standalone command and a panel
// build it the same way, through New. It keeps no state of its own.
package web

import (
	"embed"
	"io/fs"
	"net/http"

	"github.com/lsongdev/ohmyserver/modules/filemgr/api"
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

// Files is the assembled module.
type Files struct {
	API *api.Service
}

// New returns the module, showing the files under opts.Root ("/" by
// default).
func New(opts Options) (*Files, error) {
	service, err := api.New(opts)
	if err != nil {
		return nil, err
	}
	return &Files{API: service}, nil
}

// Handler serves the module's API with relative routes.
func (f *Files) Handler() http.Handler { return f.API.Handler() }

// Assets returns the browser UI; manifest.json is at its root.
func (f *Files) Assets() fs.FS { return UI }

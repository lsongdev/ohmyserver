// Package web assembles the service manager as a panel module: the JSON API
// and the browser UI (native ES modules in ui/, no build step).
//
// A Services value satisfies the panel module contract without depending
// on any panel: Handler serves the API with relative routes and Assets
// holds the UI with manifest.json at its root. The standalone command and a
// panel build it the same way, through New. It keeps no state: systemd and
// the journal are the source of truth.
package web

import (
	"embed"
	"io/fs"
	"net/http"

	"github.com/lsongdev/ohmyserver/modules/svcmgr/api"
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

// Services is the assembled module.
type Services struct {
	API *api.Service
}

// New checks for systemd. Without it the module still serves, and its
// pages say so.
func New(opts Options) *Services {
	return &Services{API: api.New(opts)}
}

// Handler serves the module's API with relative routes.
func (s *Services) Handler() http.Handler { return s.API.Handler() }

// Assets returns the browser UI; manifest.json is at its root.
func (s *Services) Assets() fs.FS { return UI }

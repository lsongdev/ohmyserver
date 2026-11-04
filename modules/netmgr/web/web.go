// Package web assembles the network manager as a panel module: the JSON API
// and the browser UI (native ES modules in ui/, no build step).
//
// A Network value satisfies the panel module contract without depending on
// any panel: Handler serves the API with relative routes and Assets holds
// the UI with manifest.json at its root. The standalone command and a panel
// build it the same way, through New. The system's network manager holds
// the settings; the module only remembers a change on trial until it is kept.
package web

import (
	"embed"
	"io/fs"
	"net/http"

	"github.com/lsongdev/ohmyserver/modules/netmgr/api"
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

// Network is the assembled module.
type Network struct {
	API *api.Service
}

// New looks for the network tools. Without them the module still serves,
// and its pages say so.
func New(opts Options) *Network {
	return &Network{API: api.New(opts)}
}

// Handler serves the module's API with relative routes.
func (n *Network) Handler() http.Handler { return n.API.Handler() }

// Assets returns the browser UI; manifest.json is at its root.
func (n *Network) Assets() fs.FS { return UI }

// Close stops waiting on a change on trial; the next start picks it up.
func (n *Network) Close() error { return n.API.Close() }

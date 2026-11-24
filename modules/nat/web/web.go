// Package web assembles the independently runnable NAT traversal module.
package web

import (
	"context"
	"embed"
	"io/fs"
	"net/http"

	"github.com/lsongdev/ohmyserver/modules/nat/api"
)

//go:embed ui
var embedded embed.FS
var UI = func() fs.FS {
	sub, err := fs.Sub(embedded, "ui")
	if err != nil {
		panic(err)
	}
	return sub
}()

type Options = api.Options
type NAT struct{ API *api.Service }

func New(ctx context.Context, opts Options) *NAT { return &NAT{API: api.New(ctx, opts)} }
func (n *NAT) Handler() http.Handler             { return n.API.Handler() }
func (n *NAT) Assets() fs.FS                     { return UI }
func (n *NAT) Close() error                      { return n.API.Close() }

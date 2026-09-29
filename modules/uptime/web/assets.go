package web

import (
	"embed"
	"github.com/lsongdev/ohmyserver/pkg/moduleweb"
	"io/fs"
	"net/http"
)

//go:embed assets
var moduleWeb embed.FS

func wrapModuleWeb(api *http.ServeMux) http.Handler {
	assets, err := fs.Sub(moduleWeb, "assets")
	if err != nil {
		panic(err)
	} // The embedded directory is checked at build time.
	return moduleweb.Wrap(api, assets)
}

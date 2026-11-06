// Package panel holds the browser runtime of the panel module contract
// (docs/module-contract.md) in one place: mount, the page context, form
// helpers, the API client, formatting, the standalone host and module.css,
// the styles every module shares. It is always served beside a
// node_modules holding @lsongdev/stylesheets, which module.css imports from
// ../node_modules: by the shell at /_shell/panel/ and by a standalone module
// at /lib/. Modules import its scripts as "@lsongdev/panel/…" through an
// import map.
package panel

import (
	"embed"
	"io/fs"
)

//go:embed ui
var embedded embed.FS

// FS is the runtime's files, at the root.
var FS = func() fs.FS {
	sub, err := fs.Sub(embedded, "ui")
	if err != nil {
		panic(err) // the embed directive guarantees the directory exists
	}
	return sub
}()

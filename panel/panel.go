// Package panel holds the browser runtime of the panel module contract
// (docs/module-contract.md) in one place: mount, the page context, form
// helpers, the API client, formatting and the standalone host. The shell
// serves it at /_panel/ and a standalone module serves it beside its own UI;
// either way modules import it as "@lsongdev/panel/…" through an import map.
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

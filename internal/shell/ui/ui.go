// Package ui embeds the shell's browser code: vendored Preact and htm, the
// layout and module host, and the components and design system the shell
// and its built-in modules use. Contract modules do not import it.
package ui

import (
	"embed"
	"io/fs"
)

//go:embed static
var embedded embed.FS

// FS is the shell's file tree.
var FS = mustSub(embedded, "static")

func mustSub(f fs.FS, dir string) fs.FS {
	sub, err := fs.Sub(f, dir)
	if err != nil {
		panic(err) // the embed directive guarantees the directory exists
	}
	return sub
}

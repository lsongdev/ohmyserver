package frontend

import (
	"embed"
	"io/fs"
)

//go:embed index.html login.html app shared vendor
var files embed.FS

func Files() fs.FS { return files }

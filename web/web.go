package web

import "embed"

//go:embed *.html **/*.html
var Templates embed.FS

//go:embed assets/**
var Assets embed.FS

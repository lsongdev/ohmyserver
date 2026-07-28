package web

import "embed"

//go:embed *.html
//go:embed */*.html
//go:embed assets/css/*.css
//go:embed assets/js/*
var Files embed.FS

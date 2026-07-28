package server

import (
	"net/http"
	"runtime"
	"strconv"
	"strings"
	"time"
)

type AboutInfo struct {
	ProgramName    string
	ProgramVersion string
	ProgramDesc    string
	GoVersion      string
	BuildTime      string
	OS             string
	Arch           string
	Listen         string
	Database       string
	DockerMirror   string
	Uptime         string
	Credits        []CreditItem
}

type CreditItem struct {
	Name    string
	URL     string
	License string
}

func (s *Server) AboutView(w http.ResponseWriter, r *http.Request) {
	info := AboutInfo{
		ProgramName:    "OhMyServer",
		ProgramVersion: "1.0.0",
		ProgramDesc:    "A modern server management platform",
		GoVersion:      runtime.Version(),
		BuildTime:      time.Now().Format("2006-01-02 15:04:05"),
		OS:             runtime.GOOS,
		Arch:           runtime.GOARCH,
		Listen:         s.config.Listen,
		Database:       s.config.Database.Driver + " - " + s.config.Database.Path,
		DockerMirror:   s.config.Docker.RegistryMirror,
		Uptime:         time.Since(time.Now()).String(),
		Credits: []CreditItem{
			{Name: "Go", URL: "https://golang.org", License: "BSD"},
			{Name: "SQLite", URL: "https://sqlite.org", License: "Public Domain"},
			{Name: "Docker", URL: "https://docker.com", License: "Apache 2.0"},
			{Name: "Font Awesome", URL: "https://fontawesome.com", License: "CC BY 4.0"},
		},
	}

	// Mask access key
	maskedKey := ""
	if len(s.config.AccessKey) > 8 {
		maskedKey = s.config.AccessKey[:4] + strings.Repeat("*", len(s.config.AccessKey)-8) + s.config.AccessKey[len(s.config.AccessKey)-4:]
	} else if s.config.AccessKey != "" {
		maskedKey = strings.Repeat("*", len(s.config.AccessKey))
	}

	s.Render(w, "about/index", H{
		"info":      info,
		"maskedKey": maskedKey,
		"numCPU":    strconv.Itoa(runtime.NumCPU()),
	})
}

package server

import (
	"context"
	"errors"
	"fmt"
	"log"
	"net/http"
	"net/url"
	"os"
	"runtime"
	"strconv"
	"strings"
	"time"

	"github.com/lsongdev/ohmyserver/buildinfo"
	"github.com/lsongdev/ohmyserver/updater"
)

const (
	updateCheckTimeout = 10 * time.Second
)

type AboutInfo struct {
	ProgramName    string
	ProgramVersion string
	ProgramCommit  string
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

type UpdateInfo struct {
	Checked        bool
	Available      bool
	Installable    bool
	LatestVersion  string
	PublishedAt    time.Time
	ReleaseURL     string
	AssetName      string
	Message        string
	ChecksumsFound bool
	PlatformFound  bool
}

type CreditItem struct {
	Name    string
	URL     string
	License string
}

func (s *Server) AboutView(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	info := AboutInfo{
		ProgramName:    "OhMyServer",
		ProgramVersion: buildinfo.Version,
		ProgramCommit:  buildinfo.ShortCommit(),
		ProgramDesc:    "A modern server management platform",
		GoVersion:      runtime.Version(),
		BuildTime:      buildinfo.BuildTime,
		OS:             runtime.GOOS,
		Arch:           runtime.GOARCH,
		Listen:         s.config.Listen,
		Database:       s.config.Database.Driver + " - " + s.config.Database.Path,
		DockerMirror:   s.config.Docker.RegistryMirror,
		Uptime:         time.Since(s.startedAt).Round(time.Second).String(),
		Credits: []CreditItem{
			{Name: "Go", URL: "https://golang.org", License: "BSD"},
			{Name: "SQLite", URL: "https://sqlite.org", License: "Public Domain"},
			{Name: "Docker", URL: "https://docker.com", License: "Apache 2.0"},
			{Name: "Font Awesome", URL: "https://fontawesome.com", License: "CC BY 4.0"},
		},
	}

	maskedKey := ""
	if len(s.config.AccessKey) > 8 {
		maskedKey = s.config.AccessKey[:4] + strings.Repeat("*", len(s.config.AccessKey)-8) + s.config.AccessKey[len(s.config.AccessKey)-4:]
	} else if s.config.AccessKey != "" {
		maskedKey = strings.Repeat("*", len(s.config.AccessKey))
	}

	var update UpdateInfo
	if r.URL.Query().Get("check") == "1" {
		update = s.checkUpdate(r.Context())
	}
	s.Render(w, "about/index", H{
		"info":          info,
		"maskedKey":     maskedKey,
		"numCPU":        strconv.Itoa(runtime.NumCPU()),
		"update":        update,
		"updateMessage": r.URL.Query().Get("message"),
	})
}

func (s *Server) checkUpdate(parent context.Context) UpdateInfo {
	ctx, cancel := context.WithTimeout(parent, updateCheckTimeout)
	defer cancel()
	info := UpdateInfo{Checked: true}
	assetName, err := updater.AssetName(runtime.GOOS, runtime.GOARCH)
	if err != nil {
		info.Message = err.Error()
		return info
	}
	info.AssetName = assetName
	release, err := updater.NewClient().Latest(ctx)
	if err != nil {
		info.Message = err.Error()
		return info
	}
	info.LatestVersion = release.TagName
	info.PublishedAt = release.PublishedAt
	info.ReleaseURL = release.HTMLURL
	_, info.PlatformFound = release.Asset(assetName)
	_, info.ChecksumsFound = release.Asset(updater.DefaultChecksumsAsset)
	info.Installable = info.PlatformFound && info.ChecksumsFound
	info.Available = buildinfo.IsDevelopment() || buildinfo.Version != release.TagName
	if !info.PlatformFound {
		info.Message = "Latest release has no binary for " + runtime.GOOS + "/" + runtime.GOARCH
	} else if !info.ChecksumsFound {
		info.Message = "Latest release has no checksums.txt and cannot be installed safely"
	} else if !info.Available {
		info.Message = "Already running the latest release"
	}
	return info
}

func (s *Server) UpdateHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	err := s.queueUpdate()
	if err != nil {
		log.Printf("Failed to queue update: %v", err)
		w.WriteHeader(http.StatusInternalServerError)
		s.Error(w, fmt.Errorf("queue update: %w", err))
		return
	}
	message := "Update queued; the service will restart after verification and installation"
	log.Printf("Update queued through %s", updater.DefaultTrigger)
	http.Redirect(w, r, "/about?message="+url.QueryEscape(message), http.StatusSeeOther)
}

func (s *Server) queueUpdate() error {
	if s.scheduleUpdate != nil {
		return s.scheduleUpdate()
	}
	trigger, err := os.OpenFile(updater.DefaultTrigger, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
	if errors.Is(err, os.ErrExist) {
		return errors.New("an update is already queued")
	}
	if err != nil {
		return errors.New("update service is not configured; run install.sh once: " + err.Error())
	}
	return trigger.Close()
}

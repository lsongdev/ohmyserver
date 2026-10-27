// Package home is OhMyServer's own module: an overview across every other
// module. It follows the module contract like any module, reads each
// module's state through its public Go API, and the modules know nothing
// about it. Being part of the shell, its UI reuses the shell's Preact and
// components instead of bundling its own.
package home

import (
	"context"
	"embed"
	"encoding/json"
	"errors"
	"io/fs"
	"net/http"
	"time"

	acmeapi "github.com/lsongdev/acme-go/api"
	dnsapi "github.com/lsongdev/dns-go/api"
	dnsweb "github.com/lsongdev/dns-go/web"
	"github.com/lsongdev/docker-go/engine"
	dockerweb "github.com/lsongdev/docker-go/web"
	forwardapi "github.com/lsongdev/forward-go/api"
	ciapi "github.com/lsongdev/ohmyserver/modules/ci/api"
	cronapi "github.com/lsongdev/ohmyserver/modules/cron/api"
	monitorapi "github.com/lsongdev/ohmyserver/modules/monitor/api"
	sshapi "github.com/lsongdev/ohmyserver/modules/ssh/api"
	"github.com/lsongdev/yabber"
)

//go:embed ui
var embedded embed.FS

var ui = func() fs.FS {
	sub, err := fs.Sub(embedded, "ui")
	if err != nil {
		panic(err) // the embed directive guarantees the directory exists
	}
	return sub
}()

// Sources are the modules the overview reports on. Nil ones are omitted.
type Sources struct {
	Docker  *dockerweb.Docker
	Certs   *acmeapi.Service
	Gateway *yabber.Server
	DNS     *dnsweb.DNS
	Forward *forwardapi.Service
	SSH     *sshapi.Service
	Cron    *cronapi.Service
	Monitor *monitorapi.Service
	CI      *ciapi.Service
}

// Home is the overview module.
type Home struct{ handler http.Handler }

// Handler serves the overview API.
func (h *Home) Handler() http.Handler { return h.handler }

// Assets returns the overview UI.
func (h *Home) Assets() fs.FS { return ui }

// New returns the overview module.
func New(src Sources) *Home {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /summary", func(w http.ResponseWriter, r *http.Request) {
		ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
		defer cancel()
		w.Header().Set("Content-Type", "application/json; charset=utf-8")
		w.Header().Set("Cache-Control", "no-store")
		json.NewEncoder(w).Encode(map[string]any{"data": src.summary(ctx)})
	})
	return &Home{handler: mux}
}

// Each section reports an error instead of failing the whole overview, so
// one unavailable module (e.g. a stopped Docker daemon) does not hide the
// others.
type dockerSummary struct {
	State       string `json:"state,omitempty"` // not_installed or not_running when the daemon is unreachable
	Error       string `json:"error,omitempty"`
	Containers  int    `json:"containers"`
	Running     int    `json:"running"`
	Apps        int    `json:"apps"`
	AppsRunning int    `json:"apps_running"`
}

type certificateSummary struct {
	Error    string `json:"error,omitempty"`
	Total    int    `json:"total"`
	Active   int    `json:"active"`
	Pending  int    `json:"pending"`
	Expiring int    `json:"expiring"` // active, expiring within 30 days
}

type gatewaySummary struct {
	Healthy   bool                    `json:"healthy"`
	Sites     int                     `json:"sites"`
	Requests  uint64                  `json:"requests"`
	Listeners []yabber.ListenerStatus `json:"listeners"`
}

type dnsSummary struct {
	Mode      string                  `json:"mode"`
	Queries   uint64                  `json:"queries"`
	Blocked   uint64                  `json:"blocked"`
	Zones     int                     `json:"zones"`
	Listeners []dnsapi.ListenerStatus `json:"listeners"`
}

func (src Sources) summary(ctx context.Context) map[string]any {
	out := map[string]any{}
	if d := src.Docker; d != nil {
		var s dockerSummary
		containers, err := d.Engine.ListContainers(ctx, engine.ListContainersOptions{All: true})
		switch {
		case engine.NotInstalled(err):
			s.State = "not_installed"
		case errors.Is(err, engine.ErrUnavailable):
			s.State, s.Error = "not_running", err.Error()
		case err != nil:
			s.Error = err.Error()
		}
		for _, c := range containers {
			s.Containers++
			if c.State == "running" {
				s.Running++
			}
		}
		if installed, err := d.Apps.List(ctx); err == nil {
			for _, a := range installed {
				s.Apps++
				if a.Status == "running" {
					s.AppsRunning++
				}
			}
		}
		out["docker"] = s
	}
	if c := src.Certs; c != nil {
		var s certificateSummary
		certificates, err := c.ListCertificates(ctx)
		if err != nil {
			s.Error = err.Error()
		}
		soon := time.Now().Add(30 * 24 * time.Hour)
		for _, cert := range certificates {
			s.Total++
			switch cert.Status {
			case acmeapi.CertificateActive:
				s.Active++
				if cert.NotAfter.Before(soon) {
					s.Expiring++
				}
			case acmeapi.CertificatePending:
				s.Pending++
			}
		}
		out["certificates"] = s
	}
	if g := src.Gateway; g != nil {
		out["gateway"] = gatewaySummary{
			Healthy: g.Healthy(), Sites: len(g.Config().Sites),
			Requests: g.Metrics().Total().Requests, Listeners: g.Status().Listeners,
		}
	}
	if d := src.DNS; d != nil {
		stats := d.Stats.Summary(0, 60)
		out["dns"] = dnsSummary{
			Mode: d.Settings().Mode, Queries: stats.Total, Blocked: stats.Blocked,
			Zones: len(d.Manager.Service().Zones.List()), Listeners: d.Listeners(),
		}
	}
	if f := src.Forward; f != nil {
		if sum, err := f.Summary(ctx); err == nil {
			out["forward"] = sum
		}
	}
	if c := src.CI; c != nil {
		if sum, err := c.Summary(ctx); err == nil {
			out["ci"] = sum
		}
	}
	if m := src.Monitor; m != nil {
		if sum, err := m.Summary(ctx); err == nil {
			out["monitor"] = sum
		}
	}
	if c := src.Cron; c != nil {
		if sum, err := c.Summary(ctx); err == nil {
			out["cron"] = sum
		}
	}
	if t := src.SSH; t != nil {
		if sum, err := t.Summary(ctx); err == nil {
			out["ssh"] = sum
		}
	}
	return out
}

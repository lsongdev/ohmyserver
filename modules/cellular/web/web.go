// Package web exposes the cellular monitor through the panel module contract.
package web

import (
	"embed"
	"io/fs"
	"net/http"

	"github.com/lsongdev/ohmyserver/modules/cellular/internal/modem"
)

//go:embed ui
var embedded embed.FS

type Cellular struct {
	monitor       *modem.Monitor
	authenticated bool
}

// Options describes the serial port and whether the host enforces sign-in.
type Options struct {
	Port          string
	Authenticated bool
}

func New(opts Options) *Cellular {
	return &Cellular{monitor: modem.Start(opts.Port), authenticated: opts.Authenticated}
}

func (m *Cellular) Close() error {
	m.monitor.Close()
	return nil
}

func (m *Cellular) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /status", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, http.StatusOK, m.monitor.Snapshot())
	})
	mux.HandleFunc("GET /sms", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, http.StatusOK, map[string]bool{"available": m.authenticated})
	})
	mux.HandleFunc("POST /sms/list", m.requireSMS(m.listSMS))
	mux.HandleFunc("POST /sms/send", m.requireSMS(m.sendSMS))
	mux.HandleFunc("GET /at", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, http.StatusOK, map[string]bool{"available": m.authenticated})
	})
	mux.HandleFunc("POST /at", m.command)
	return mux
}

func (m *Cellular) Assets() fs.FS {
	ui, err := fs.Sub(embedded, "ui")
	if err != nil {
		panic(err)
	}
	return ui
}

package server

import (
	"log"
	"net/http"
	"net/url"
	"strconv"

	"github.com/gorilla/websocket"
	terminalpkg "github.com/lsongdev/ohmyserver/terminal"
)

var terminalUpgrader = websocket.Upgrader{
	CheckOrigin: func(r *http.Request) bool {
		origin := r.Header.Get("Origin")
		if origin == "" {
			return true
		}
		u, err := url.Parse(origin)
		return err == nil && u.Host == r.Host
	},
}

func (s *Server) TerminalHandler(w http.ResponseWriter, r *http.Request) {
	s.renderTerminal(w, "Terminal", "/terminal/local/ws", "")
}

func (s *Server) renderTerminal(w http.ResponseWriter, title, wsPath, host string) {
	s.Render(w, "terminal/index", H{
		"title":  title,
		"wsPath": wsPath,
		"host":   host,
	})
}

func (s *Server) TerminalWS(w http.ResponseWriter, r *http.Request) {
	s.serveTerminal(w, r, terminalpkg.Local{})
}

func (s *Server) serveTerminal(w http.ResponseWriter, r *http.Request, backend terminalpkg.Backend) {
	conn, err := terminalUpgrader.Upgrade(w, r, nil)
	if err != nil {
		return
	}

	cols, rows := terminalSize(r)
	session, err := backend.Open(r.Context(), cols, rows)
	if err != nil {
		_ = conn.WriteMessage(websocket.TextMessage, terminalpkg.ErrorMessage(err))
		_ = conn.Close()
		return
	}

	if err := terminalpkg.Bridge(r.Context(), conn, session); err != nil {
		log.Printf("terminal session: %v", err)
	}
}

func terminalSize(r *http.Request) (uint16, uint16) {
	cols := parseTerminalDimension(r.URL.Query().Get("cols"), 80)
	rows := parseTerminalDimension(r.URL.Query().Get("rows"), 24)
	return cols, rows
}

func parseTerminalDimension(value string, fallback uint16) uint16 {
	n, err := strconv.ParseUint(value, 10, 16)
	if err != nil || n == 0 {
		return fallback
	}
	return uint16(n)
}

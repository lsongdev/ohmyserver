package api

import (
	"encoding/json"
	"errors"
	"net/http"
	"strconv"

	"github.com/lsongdev/ohmyserver/modules/notify/store"
)

// Handler serves the API. Successful responses are {"data": ...}; failures
// are {"error": {"code": ..., "message": ...}} with a matching status.
func (s *Service) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /events", func(w http.ResponseWriter, r *http.Request) {
		q := r.URL.Query()
		limit, _ := strconv.Atoi(q.Get("limit"))
		before, _ := strconv.ParseInt(q.Get("before"), 10, 64)
		list, err := s.Events(r.Context(), store.EventQuery{
			Level: q.Get("level"), Source: q.Get("source"), Search: q.Get("q"), Before: before, Limit: limit,
		})
		respond(w, list, err)
	})
	mux.HandleFunc("GET /sources", func(w http.ResponseWriter, r *http.Request) {
		list, err := s.Sources(r.Context())
		respond(w, list, err)
	})
	mux.HandleFunc("GET /issues", func(w http.ResponseWriter, r *http.Request) {
		state := r.URL.Query().Get("state")
		if state == "" {
			state = store.StateOpen
		}
		if state != store.StateOpen && state != store.StateResolved {
			writeError(w, http.StatusBadRequest, "invalid_request", "state must be open or resolved")
			return
		}
		list, err := s.Issues(r.Context(), state)
		respond(w, list, err)
	})
	mux.HandleFunc("POST /issues/{id}/resolve", func(w http.ResponseWriter, r *http.Request) {
		id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
		if err != nil {
			writeError(w, http.StatusBadRequest, "invalid_request", "invalid issue id")
			return
		}
		respond(w, nil, s.CloseIssue(r.Context(), id))
	})
	mux.HandleFunc("GET /summary", func(w http.ResponseWriter, r *http.Request) {
		sum, err := s.Summary(r.Context())
		respond(w, sum, err)
	})
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		writeError(w, http.StatusNotFound, "not_found", "no such endpoint")
	})
	return mux
}

func respond(w http.ResponseWriter, data any, err error) {
	switch {
	case err == nil:
		writeJSON(w, http.StatusOK, map[string]any{"data": data})
	case errors.Is(err, ErrInvalid):
		writeError(w, http.StatusBadRequest, "invalid_request", err.Error())
	case errors.Is(err, store.ErrNotFound):
		writeError(w, http.StatusNotFound, "not_found", "no such issue")
	default:
		writeError(w, http.StatusInternalServerError, "internal", err.Error())
	}
}

func writeError(w http.ResponseWriter, status int, code, message string) {
	writeJSON(w, status, map[string]any{"error": map[string]string{"code": code, "message": message}})
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(v)
}

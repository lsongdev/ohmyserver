package api

import (
	"encoding/json"
	"errors"
	"net/http"
	"strconv"

	"github.com/lsongdev/ohmyserver/modules/pkgmgr/system"
)

// Handler serves the API. Successful responses are {"data": ...}; failures
// are {"error": {"code": ..., "message": ...}} with a matching status.
func (s *Service) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /system", func(w http.ResponseWriter, r *http.Request) {
		respond(w, http.StatusOK, s.Info(), nil)
	})
	mux.HandleFunc("GET /installed", func(w http.ResponseWriter, r *http.Request) {
		v, err := s.Installed(r.Context())
		respond(w, http.StatusOK, nonNil(v), err)
	})
	mux.HandleFunc("GET /updates", func(w http.ResponseWriter, r *http.Request) {
		v, err := s.Updates(r.Context())
		respond(w, http.StatusOK, nonNil(v), err)
	})
	mux.HandleFunc("GET /search", func(w http.ResponseWriter, r *http.Request) {
		v, err := s.Search(r.Context(), r.URL.Query().Get("q"))
		respond(w, http.StatusOK, nonNil(v), err)
	})
	mux.HandleFunc("GET /info", func(w http.ResponseWriter, r *http.Request) {
		v, err := s.PackageInfo(r.Context(), r.URL.Query().Get("name"))
		respond(w, http.StatusOK, map[string]string{"text": v}, err)
	})
	mux.HandleFunc("GET /docker", func(w http.ResponseWriter, r *http.Request) {
		respond(w, http.StatusOK, s.DockerStatus(r.Context()), nil)
	})
	mux.HandleFunc("GET /jobs", func(w http.ResponseWriter, r *http.Request) {
		respond(w, http.StatusOK, s.Jobs(), nil)
	})
	mux.HandleFunc("POST /jobs", func(w http.ResponseWriter, r *http.Request) {
		var req JobRequest
		if decode(w, r, &req) {
			job, err := s.Start(req)
			respond(w, http.StatusAccepted, job, err)
		}
	})
	mux.HandleFunc("GET /jobs/{id}", func(w http.ResponseWriter, r *http.Request) {
		id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
		if err != nil {
			writeError(w, http.StatusBadRequest, "invalid_request", "invalid job id")
			return
		}
		job, err := s.Job(id)
		respond(w, http.StatusOK, job, err)
	})
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		writeError(w, http.StatusNotFound, "not_found", "no such endpoint")
	})
	return mux
}

func nonNil[T any](items []T) []T {
	if items == nil {
		return []T{}
	}
	return items
}

func respond(w http.ResponseWriter, status int, data any, err error) {
	switch {
	case err == nil && status == http.StatusNoContent:
		w.WriteHeader(status)
	case err == nil:
		writeJSON(w, status, map[string]any{"data": data})
	case errors.Is(err, system.ErrInvalid):
		writeError(w, http.StatusBadRequest, "invalid_request", err.Error())
	case errors.Is(err, ErrNotFound):
		writeError(w, http.StatusNotFound, "not_found", err.Error())
	case errors.Is(err, ErrBusy):
		writeError(w, http.StatusConflict, "busy", "Another job is running: wait for it to finish")
	case errors.Is(err, system.ErrUnsupported):
		writeError(w, http.StatusServiceUnavailable, "unsupported", "No supported package manager (dnf, apt or brew) on this system")
	default:
		writeError(w, http.StatusBadGateway, "command_failed", err.Error())
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

func decode(w http.ResponseWriter, r *http.Request, v any) bool {
	dec := json.NewDecoder(http.MaxBytesReader(w, r.Body, 64<<10))
	dec.DisallowUnknownFields()
	if err := dec.Decode(v); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_request", "invalid JSON: "+err.Error())
		return false
	}
	return true
}

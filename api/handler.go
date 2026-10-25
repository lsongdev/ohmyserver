package api

import (
	"encoding/json"
	"errors"
	"net/http"
)

// Handler serves the API. Successful responses are {"data": ...}; failures
// are {"error": {"code": ..., "message": ...}} with a matching status.
func (s *Service) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /hosts", func(w http.ResponseWriter, r *http.Request) {
		hosts, err := s.Hosts(r.Context(), r.URL.Query().Get("spark") != "")
		respond(w, http.StatusOK, hosts, err)
	})
	mux.HandleFunc("GET /hosts/{id}", func(w http.ResponseWriter, r *http.Request) {
		h, err := s.Host(r.Context(), r.PathValue("id"))
		respond(w, http.StatusOK, h, err)
	})
	mux.HandleFunc("GET /hosts/{id}/series", func(w http.ResponseWriter, r *http.Request) {
		period, ok := Ranges[r.URL.Query().Get("range")]
		if !ok {
			writeError(w, http.StatusBadRequest, "invalid_request", "range must be one of 1h, 6h, 24h, 7d, 30d, 90d")
			return
		}
		samples, step, err := s.Series(r.Context(), r.PathValue("id"), period)
		respond(w, http.StatusOK, map[string]any{"step": int64(step.Seconds()), "points": samples}, err)
	})
	for action, enabled := range map[string]bool{"enable": true, "disable": false} {
		mux.HandleFunc("POST /hosts/{id}/"+action, func(w http.ResponseWriter, r *http.Request) {
			h, err := s.SetEnabled(r.Context(), r.PathValue("id"), enabled)
			respond(w, http.StatusOK, h, err)
		})
	}
	mux.HandleFunc("DELETE /hosts/{id}", func(w http.ResponseWriter, r *http.Request) {
		err := s.DeleteHost(r.Context(), r.PathValue("id"))
		if err != nil && !errors.Is(err, ErrNotFound) {
			writeError(w, http.StatusConflict, "conflict", err.Error())
			return
		}
		respond(w, http.StatusNoContent, nil, err)
	})
	mux.HandleFunc("GET /summary", func(w http.ResponseWriter, r *http.Request) {
		sum, err := s.Summary(r.Context())
		respond(w, http.StatusOK, sum, err)
	})
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		writeError(w, http.StatusNotFound, "not_found", "no such API endpoint")
	})
	return mux
}

func respond(w http.ResponseWriter, status int, data any, err error) {
	switch {
	case err == nil:
	case errors.Is(err, ErrNotFound):
		writeError(w, http.StatusNotFound, "not_found", err.Error())
		return
	default:
		writeError(w, http.StatusInternalServerError, "internal", err.Error())
		return
	}
	if status == http.StatusNoContent || data == nil {
		w.WriteHeader(status)
		return
	}
	writeJSON(w, status, map[string]any{"data": data})
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

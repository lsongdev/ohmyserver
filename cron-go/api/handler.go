package api

import (
	"encoding/json"
	"errors"
	"net/http"
	"strconv"

	"github.com/lsongdev/cron-go/store"
)

// Handler serves the API. Successful responses are {"data": ...}; failures
// are {"error": {"code": ..., "message": ...}} with a matching status.
func (s *Service) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /jobs", func(w http.ResponseWriter, r *http.Request) {
		jobs, err := s.Jobs(r.Context())
		respond(w, http.StatusOK, jobs, err)
	})
	mux.HandleFunc("POST /jobs", func(w http.ResponseWriter, r *http.Request) {
		var j store.Job
		if decode(w, r, &j) {
			v, err := s.CreateJob(r.Context(), j)
			respond(w, http.StatusCreated, v, err)
		}
	})
	mux.HandleFunc("GET /jobs/{id}", func(w http.ResponseWriter, r *http.Request) {
		if id, ok := pathID(w, r); ok {
			v, err := s.Job(r.Context(), id)
			respond(w, http.StatusOK, v, err)
		}
	})
	mux.HandleFunc("PUT /jobs/{id}", func(w http.ResponseWriter, r *http.Request) {
		var j store.Job
		if id, ok := pathID(w, r); ok && decode(w, r, &j) {
			v, err := s.UpdateJob(r.Context(), id, j)
			respond(w, http.StatusOK, v, err)
		}
	})
	mux.HandleFunc("DELETE /jobs/{id}", func(w http.ResponseWriter, r *http.Request) {
		if id, ok := pathID(w, r); ok {
			respond(w, http.StatusNoContent, nil, s.DeleteJob(r.Context(), id))
		}
	})
	for action, enabled := range map[string]bool{"enable": true, "disable": false} {
		mux.HandleFunc("POST /jobs/{id}/"+action, func(w http.ResponseWriter, r *http.Request) {
			if id, ok := pathID(w, r); ok {
				v, err := s.SetEnabled(r.Context(), id, enabled)
				respond(w, http.StatusOK, v, err)
			}
		})
	}
	mux.HandleFunc("POST /jobs/{id}/run", func(w http.ResponseWriter, r *http.Request) {
		if id, ok := pathID(w, r); ok {
			run, err := s.RunNow(r.Context(), id)
			respond(w, http.StatusAccepted, run, err)
		}
	})
	mux.HandleFunc("GET /jobs/{id}/runs", func(w http.ResponseWriter, r *http.Request) {
		if id, ok := pathID(w, r); ok {
			runs, err := s.Runs(r.Context(), id)
			respond(w, http.StatusOK, runs, err)
		}
	})
	mux.HandleFunc("GET /runs/{id}", func(w http.ResponseWriter, r *http.Request) {
		if id, ok := pathID(w, r); ok {
			run, err := s.Run(r.Context(), id)
			respond(w, http.StatusOK, run, err)
		}
	})
	mux.HandleFunc("POST /runs/{id}/cancel", func(w http.ResponseWriter, r *http.Request) {
		if id, ok := pathID(w, r); ok {
			respond(w, http.StatusAccepted, nil, s.Cancel(r.Context(), id))
		}
	})
	mux.HandleFunc("GET /targets", func(w http.ResponseWriter, r *http.Request) {
		targets, err := s.Targets(r.Context())
		respond(w, http.StatusOK, targets, err)
	})
	mux.HandleFunc("GET /preview", func(w http.ResponseWriter, r *http.Request) {
		q := r.URL.Query()
		times, err := Preview(q.Get("schedule"), q.Get("timezone"), 5)
		respond(w, http.StatusOK, map[string]any{"next": times}, err)
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
	case errors.Is(err, ErrInvalid):
		writeError(w, http.StatusUnprocessableEntity, "invalid_request", err.Error())
		return
	case errors.Is(err, store.ErrNotFound):
		writeError(w, http.StatusNotFound, "not_found", err.Error())
		return
	case errors.Is(err, store.ErrConflict), errors.Is(err, ErrRunning):
		writeError(w, http.StatusConflict, "conflict", err.Error())
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

func decode(w http.ResponseWriter, r *http.Request, v any) bool {
	dec := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<20))
	dec.DisallowUnknownFields()
	if err := dec.Decode(v); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_request", "invalid JSON body: "+err.Error())
		return false
	}
	return true
}

func pathID(w http.ResponseWriter, r *http.Request) (int64, bool) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil || id < 1 {
		writeError(w, http.StatusBadRequest, "invalid_request", "invalid id")
		return 0, false
	}
	return id, true
}

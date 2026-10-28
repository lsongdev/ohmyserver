package api

import (
	"encoding/json"
	"errors"
	"net/http"
	"strconv"

	"github.com/lsongdev/ohmyserver/modules/domains/provider"
	"github.com/lsongdev/ohmyserver/modules/domains/store"
)

// Handler serves the API. Successful responses are {"data": ...}; failures
// are {"error": {"code": ..., "message": ...}} with a matching status.
func (s *Service) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /kinds", func(w http.ResponseWriter, r *http.Request) {
		respond(w, http.StatusOK, provider.Kinds(), nil)
	})
	mux.HandleFunc("GET /accounts", func(w http.ResponseWriter, r *http.Request) {
		list, err := s.Accounts(r.Context())
		respond(w, http.StatusOK, list, err)
	})
	mux.HandleFunc("POST /accounts", func(w http.ResponseWriter, r *http.Request) {
		var in AccountInput
		if decode(w, r, &in) {
			a, err := s.CreateAccount(r.Context(), in)
			respond(w, http.StatusCreated, a, err)
		}
	})
	mux.HandleFunc("POST /accounts/test", func(w http.ResponseWriter, r *http.Request) {
		var in struct {
			ID int64 `json:"id"`
			AccountInput
		}
		if decode(w, r, &in) {
			zones, err := s.Test(r.Context(), in.ID, in.AccountInput)
			respond(w, http.StatusOK, zones, err)
		}
	})
	mux.HandleFunc("GET /accounts/{id}", func(w http.ResponseWriter, r *http.Request) {
		if id, ok := pathID(w, r); ok {
			a, err := s.Account(r.Context(), id)
			respond(w, http.StatusOK, a, err)
		}
	})
	mux.HandleFunc("PUT /accounts/{id}", func(w http.ResponseWriter, r *http.Request) {
		var in AccountInput
		if id, ok := pathID(w, r); ok && decode(w, r, &in) {
			a, err := s.UpdateAccount(r.Context(), id, in)
			respond(w, http.StatusOK, a, err)
		}
	})
	mux.HandleFunc("DELETE /accounts/{id}", func(w http.ResponseWriter, r *http.Request) {
		if id, ok := pathID(w, r); ok {
			respond(w, http.StatusNoContent, nil, s.DeleteAccount(r.Context(), id))
		}
	})
	mux.HandleFunc("GET /zones", func(w http.ResponseWriter, r *http.Request) {
		zones, err := s.Zones(r.Context(), r.URL.Query().Get("refresh") == "1")
		respond(w, http.StatusOK, zones, err)
	})
	mux.HandleFunc("GET /accounts/{id}/zones/{zone}", func(w http.ResponseWriter, r *http.Request) {
		if id, ok := pathID(w, r); ok {
			z, err := s.Zone(r.Context(), id, r.PathValue("zone"))
			respond(w, http.StatusOK, z, err)
		}
	})
	mux.HandleFunc("POST /accounts/{id}/zones/{zone}/records", func(w http.ResponseWriter, r *http.Request) {
		var rec provider.Record
		if id, ok := pathID(w, r); ok && decode(w, r, &rec) {
			created, err := s.CreateRecord(r.Context(), id, r.PathValue("zone"), rec)
			respond(w, http.StatusCreated, created, err)
		}
	})
	mux.HandleFunc("PUT /accounts/{id}/zones/{zone}/records/{record}", func(w http.ResponseWriter, r *http.Request) {
		var rec provider.Record
		if id, ok := pathID(w, r); ok && decode(w, r, &rec) {
			updated, err := s.UpdateRecord(r.Context(), id, r.PathValue("zone"), r.PathValue("record"), rec)
			respond(w, http.StatusOK, updated, err)
		}
	})
	mux.HandleFunc("DELETE /accounts/{id}/zones/{zone}/records/{record}", func(w http.ResponseWriter, r *http.Request) {
		if id, ok := pathID(w, r); ok {
			respond(w, http.StatusNoContent, nil, s.DeleteRecord(r.Context(), id, r.PathValue("zone"), r.PathValue("record")))
		}
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
	var pe *ProviderError
	switch {
	case err == nil:
	case errors.Is(err, ErrInvalid):
		writeError(w, http.StatusUnprocessableEntity, "invalid_request", err.Error())
		return
	case errors.Is(err, store.ErrNotFound), errors.Is(err, ErrNotFound):
		writeError(w, http.StatusNotFound, "not_found", err.Error())
		return
	case errors.Is(err, store.ErrConflict):
		writeError(w, http.StatusConflict, "conflict", err.Error())
		return
	case errors.As(err, &pe):
		writeError(w, http.StatusBadGateway, "provider", pe.Error())
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

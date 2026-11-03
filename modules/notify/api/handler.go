package api

import (
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
	"time"

	"github.com/lsongdev/ohmyserver/modules/notify/channels"
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
	mux.HandleFunc("POST /issues/{id}/mute", func(w http.ResponseWriter, r *http.Request) {
		id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
		if err != nil {
			writeError(w, http.StatusBadRequest, "invalid_request", "invalid issue id")
			return
		}
		var body struct {
			Minutes int `json:"minutes"` // 0 ends it
		}
		if err := decode(w, r, &body); err != nil {
			return
		}
		if body.Minutes < 0 || body.Minutes > 60*24*365 {
			writeError(w, http.StatusBadRequest, "invalid_request", "minutes out of range")
			return
		}
		var until *time.Time
		if body.Minutes > 0 {
			at := s.now().Add(time.Duration(body.Minutes) * time.Minute)
			until = &at
		}
		respond(w, nil, s.MuteIssue(r.Context(), id, until))
	})
	mux.HandleFunc("GET /info", func(w http.ResponseWriter, r *http.Request) {
		respond(w, map[string]bool{"unauthenticated": s.open}, nil)
	})
	mux.HandleFunc("GET /channels", func(w http.ResponseWriter, r *http.Request) {
		list, err := s.Channels(r.Context())
		respond(w, list, err)
	})
	mux.HandleFunc("POST /channels", func(w http.ResponseWriter, r *http.Request) {
		var in ChannelInput
		if err := decode(w, r, &in); err != nil {
			return
		}
		view, err := s.CreateChannel(r.Context(), in)
		respond(w, view, err)
	})
	withID := func(fn func(w http.ResponseWriter, r *http.Request, id int64)) http.HandlerFunc {
		return func(w http.ResponseWriter, r *http.Request) {
			id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
			if err != nil {
				writeError(w, http.StatusBadRequest, "invalid_request", "invalid id")
				return
			}
			fn(w, r, id)
		}
	}
	mux.HandleFunc("GET /channels/{id}", withID(func(w http.ResponseWriter, r *http.Request, id int64) {
		view, err := s.Channel(r.Context(), id)
		respond(w, view, err)
	}))
	mux.HandleFunc("PUT /channels/{id}", withID(func(w http.ResponseWriter, r *http.Request, id int64) {
		var in ChannelInput
		if err := decode(w, r, &in); err != nil {
			return
		}
		view, err := s.UpdateChannel(r.Context(), id, in)
		respond(w, view, err)
	}))
	mux.HandleFunc("DELETE /channels/{id}", withID(func(w http.ResponseWriter, r *http.Request, id int64) {
		respond(w, nil, s.DeleteChannel(r.Context(), id))
	}))
	mux.HandleFunc("POST /channels/{id}/test", withID(func(w http.ResponseWriter, r *http.Request, id int64) {
		respond(w, nil, s.TestChannel(r.Context(), id))
	}))
	mux.HandleFunc("GET /deliveries", func(w http.ResponseWriter, r *http.Request) {
		q := r.URL.Query()
		channel, _ := strconv.ParseInt(q.Get("channel"), 10, 64)
		before, _ := strconv.ParseInt(q.Get("before"), 10, 64)
		list, err := s.Deliveries(r.Context(), store.DeliveryQuery{Channel: channel, State: q.Get("state"), Before: before, Limit: 100})
		respond(w, list, err)
	})
	mux.HandleFunc("POST /deliveries/{id}/retry", withID(func(w http.ResponseWriter, r *http.Request, id int64) {
		respond(w, nil, s.RetryDelivery(r.Context(), id))
	}))
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
	case errors.Is(err, store.ErrNameTaken):
		writeError(w, http.StatusConflict, "conflict", err.Error())
	case errors.Is(err, ErrSendFailed):
		writeError(w, http.StatusBadGateway, "send_failed", err.Error())
	case errors.Is(err, ErrInvalid), errors.Is(err, channels.ErrInvalid):
		writeError(w, http.StatusBadRequest, "invalid_request", err.Error())
	case errors.Is(err, store.ErrNotFound):
		writeError(w, http.StatusNotFound, "not_found", "not found")
	default:
		writeError(w, http.StatusInternalServerError, "internal", err.Error())
	}
}

// decode reads a JSON body, answering for itself when it cannot.
func decode(w http.ResponseWriter, r *http.Request, v any) error {
	err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<16)).Decode(v)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid_request", "invalid JSON: "+err.Error())
	}
	return err
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

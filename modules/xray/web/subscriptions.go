package web

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"

	"github.com/lsongdev/ohmyserver/modules/xray/engine"
)

func (m *Module) subscriptionRoutes(mux *http.ServeMux) {
	mux.HandleFunc("GET /subscriptions", func(w http.ResponseWriter, r *http.Request) {
		list, revision := m.engine.Subscriptions()
		reply(w, 200, map[string]any{"data": map[string]any{"subscriptions": list, "revision": revision}})
	})
	mux.HandleFunc("GET /subscriptions/{id}", func(w http.ResponseWriter, r *http.Request) {
		s, revision, err := m.engine.Subscription(r.PathValue("id"))
		if err != nil {
			fail(w, 404, "subscription not found")
			return
		}
		reply(w, 200, map[string]any{"data": map[string]any{"subscription": s, "revision": revision}})
	})
	for _, route := range []string{"POST /subscriptions", "PUT /subscriptions/{id}", "POST /subscriptions/{id}/refresh", "POST /subscriptions/{id}/import", "POST /subscriptions/{id}/delete"} {
		mux.HandleFunc(route, func(w http.ResponseWriter, r *http.Request) {
			var req struct {
				Revision *uint64  `json:"revision"`
				Name     string   `json:"name"`
				URL      string   `json:"url"`
				IDs      []string `json:"ids"`
			}
			d := json.NewDecoder(http.MaxBytesReader(w, r.Body, 2<<20))
			d.DisallowUnknownFields()
			if d.Decode(&req) != nil || req.Revision == nil || d.Decode(new(any)) != io.EOF {
				fail(w, 400, "expected one JSON object with a revision")
				return
			}
			var data any
			var err error
			switch r.Pattern {
			case "POST /subscriptions", "PUT /subscriptions/{id}":
				data, err = m.engine.SaveSubscription(*req.Revision, r.PathValue("id"), req.Name, req.URL)
			case "POST /subscriptions/{id}/refresh":
				data, err = m.engine.Refresh(r.Context(), r.PathValue("id"), *req.Revision)
			case "POST /subscriptions/{id}/import":
				data, err = m.engine.ImportSubscription(*req.Revision, r.PathValue("id"), req.IDs)
			case "POST /subscriptions/{id}/delete":
				data, err = m.engine.DeleteSubscription(*req.Revision, r.PathValue("id"))
			}
			if err != nil {
				status := 500
				if errors.Is(err, engine.ErrInvalid) {
					status = 422
				}
				if errors.Is(err, engine.ErrConflict) {
					status = 409
				}
				fail(w, status, err.Error())
				return
			}
			reply(w, 200, map[string]any{"data": data})
		})
	}
}

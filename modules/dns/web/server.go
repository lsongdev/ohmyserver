package web

import (
	"context"
	"encoding/json"
	"errors"
	dnsmodule "github.com/lsongdev/ohmyserver/modules/dns"
	"net/http"
	"strconv"
)

type Service interface {
	UpdateDNSConfiguration(context.Context, dnsmodule.Settings) error
	DNSRuntimeStatus(context.Context) (any, error)
	DNSAction(context.Context, string) error
	ReloadDNS(context.Context) error
}

type ProviderZone = dnsmodule.ProviderZone

type ProviderRecord = dnsmodule.ProviderRecord
type Module struct {
	repo    *Repository
	service Service
	listDir string
}

func New(repo *Repository, service Service, listDir string) *Module {
	return &Module{repo: repo, service: service, listDir: listDir}
}
func (*Module) Name() string { return "dns" }

func (m *Module) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /settings", func(w http.ResponseWriter, r *http.Request) {
		item, err := m.repo.GetSettings(r.Context())
		if err != nil {
			writeError(w, err)
			return
		}
		writeJSON(w, 200, map[string]any{"data": item})
	})
	mux.HandleFunc("PUT /settings", func(w http.ResponseWriter, r *http.Request) {
		var item dnsmodule.Settings
		if json.NewDecoder(r.Body).Decode(&item) != nil {
			writeProblem(w, 400, "invalid_body", "invalid JSON settings object")
			return
		}
		if err := m.service.UpdateDNSConfiguration(r.Context(), item); err != nil {
			writeError(w, err)
			return
		}
		writeJSON(w, 200, map[string]any{"data": item})
	})
	mux.HandleFunc("GET /status", func(w http.ResponseWriter, r *http.Request) {
		status, err := m.service.DNSRuntimeStatus(r.Context())
		if err != nil {
			writeError(w, err)
			return
		}
		writeJSON(w, 200, map[string]any{"data": status})
	})
	mux.HandleFunc("POST /action", func(w http.ResponseWriter, r *http.Request) {
		var input struct {
			Action string `json:"action"`
		}
		if json.NewDecoder(r.Body).Decode(&input) != nil {
			writeProblem(w, 400, "invalid_body", "invalid JSON action")
			return
		}
		if err := m.service.DNSAction(r.Context(), input.Action); err != nil {
			writeError(w, err)
			return
		}
		status, err := m.service.DNSRuntimeStatus(r.Context())
		if err != nil {
			writeError(w, err)
			return
		}
		writeJSON(w, 200, map[string]any{"data": status})
	})
	mux.HandleFunc("GET /providers", func(w http.ResponseWriter, r *http.Request) {
		items, err := m.repo.ListSummaries(r.Context())
		if err != nil {
			writeError(w, err)
			return
		}
		writeJSON(w, 200, map[string]any{"data": items})
	})
	mux.HandleFunc("POST /providers", func(w http.ResponseWriter, r *http.Request) {
		var input struct {
			Name   string          `json:"name"`
			Type   string          `json:"type"`
			Config json.RawMessage `json:"config"`
		}
		if json.NewDecoder(r.Body).Decode(&input) != nil || input.Name == "" || input.Type == "" || len(input.Config) == 0 || !json.Valid(input.Config) {
			writeProblem(w, 400, "invalid_provider", "name, type and a JSON config are required")
			return
		}
		item, err := m.repo.Create(r.Context(), input.Name, input.Type, string(input.Config))
		if err != nil {
			writeError(w, err)
			return
		}
		writeJSON(w, 201, map[string]any{"data": summary(*item)})
	})
	mux.HandleFunc("PUT /providers/{id}", func(w http.ResponseWriter, r *http.Request) {
		id, err := pathID(r)
		if err != nil {
			writeProblem(w, 400, "invalid_id", "provider id must be a positive integer")
			return
		}
		var input struct {
			Name   string          `json:"name"`
			Type   string          `json:"type"`
			Config json.RawMessage `json:"config"`
		}
		if json.NewDecoder(r.Body).Decode(&input) != nil || input.Name == "" || input.Type == "" || len(input.Config) == 0 || !json.Valid(input.Config) {
			writeProblem(w, 400, "invalid_provider", "name, type and a JSON config are required")
			return
		}
		if err := m.repo.Update(r.Context(), id, input.Name, input.Type, string(input.Config)); err != nil {
			writeError(w, err)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	})
	mux.HandleFunc("DELETE /providers/{id}", func(w http.ResponseWriter, r *http.Request) {
		id, err := pathID(r)
		if err != nil {
			writeProblem(w, 400, "invalid_id", "provider id must be a positive integer")
			return
		}
		if err := m.repo.Delete(r.Context(), id); err != nil {
			writeError(w, err)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	})
	mux.HandleFunc("GET /providers/{id}/zones", func(w http.ResponseWriter, r *http.Request) {
		id, err := pathID(r)
		if err != nil {
			writeProblem(w, 400, "invalid_id", "provider id must be a positive integer")
			return
		}
		items, err := m.providerZones(r.Context(), id)
		if err != nil {
			writeError(w, err)
			return
		}
		writeJSON(w, 200, map[string]any{"data": items})
	})
	mux.HandleFunc("GET /providers/{id}/zones/{zone}/records", func(w http.ResponseWriter, r *http.Request) {
		id, err := pathID(r)
		if err != nil {
			writeProblem(w, 400, "invalid_id", "provider id must be a positive integer")
			return
		}
		items, err := m.providerRecords(r.Context(), id, r.PathValue("zone"))
		if err != nil {
			writeError(w, err)
			return
		}
		writeJSON(w, 200, map[string]any{"data": items})
	})
	mux.HandleFunc("POST /providers/{id}/zones/{zone}/records", func(w http.ResponseWriter, r *http.Request) {
		id, err := pathID(r)
		if err != nil {
			writeProblem(w, 400, "invalid_id", "provider id must be a positive integer")
			return
		}
		var item ProviderRecord
		if json.NewDecoder(r.Body).Decode(&item) != nil {
			writeProblem(w, 400, "invalid_body", "invalid provider DNS record")
			return
		}
		item, err = m.saveProviderRecord(r.Context(), id, r.PathValue("zone"), item)
		if err != nil {
			writeError(w, err)
			return
		}
		writeJSON(w, 201, map[string]any{"data": item})
	})
	mux.HandleFunc("PUT /providers/{id}/zones/{zone}/records/{record}", func(w http.ResponseWriter, r *http.Request) {
		id, err := pathID(r)
		if err != nil {
			writeProblem(w, 400, "invalid_id", "provider id must be a positive integer")
			return
		}
		var item ProviderRecord
		if json.NewDecoder(r.Body).Decode(&item) != nil {
			writeProblem(w, 400, "invalid_body", "invalid provider DNS record")
			return
		}
		item.ID = r.PathValue("record")
		if _, err := m.saveProviderRecord(r.Context(), id, r.PathValue("zone"), item); err != nil {
			writeError(w, err)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	})
	mux.HandleFunc("DELETE /providers/{id}/zones/{zone}/records/{record}", func(w http.ResponseWriter, r *http.Request) {
		id, err := pathID(r)
		if err != nil {
			writeProblem(w, 400, "invalid_id", "provider id must be a positive integer")
			return
		}
		if err := m.deleteProviderRecord(r.Context(), id, r.PathValue("zone"), r.PathValue("record")); err != nil {
			writeError(w, err)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	})
	mux.HandleFunc("GET /zones", func(w http.ResponseWriter, r *http.Request) {
		items, err := m.repo.Zones(r.Context())
		if err != nil {
			writeError(w, err)
			return
		}
		writeJSON(w, 200, map[string]any{"data": items})
	})
	mux.HandleFunc("GET /subscriptions", func(w http.ResponseWriter, r *http.Request) {
		items, err := m.repo.Subscriptions(r.Context())
		if err != nil {
			writeError(w, err)
			return
		}
		writeJSON(w, 200, map[string]any{"data": items})
	})
	mux.HandleFunc("POST /subscriptions", func(w http.ResponseWriter, r *http.Request) {
		var item dnsmodule.Subscription
		if json.NewDecoder(r.Body).Decode(&item) != nil {
			writeProblem(w, 400, "invalid_body", "invalid JSON subscription")
			return
		}
		if err := m.saveSubscription(r.Context(), &item); err != nil {
			writeError(w, err)
			return
		}
		writeJSON(w, 201, map[string]any{"data": item})
	})
	mux.HandleFunc("PUT /subscriptions/{id}", func(w http.ResponseWriter, r *http.Request) {
		id, err := pathID(r)
		if err != nil {
			writeProblem(w, 400, "invalid_id", "subscription id must be a positive integer")
			return
		}
		var item dnsmodule.Subscription
		if json.NewDecoder(r.Body).Decode(&item) != nil {
			writeProblem(w, 400, "invalid_body", "invalid JSON subscription")
			return
		}
		item.ID = id
		if err := m.saveSubscription(r.Context(), &item); err != nil {
			writeError(w, err)
			return
		}
		writeJSON(w, 200, map[string]any{"data": item})
	})
	mux.HandleFunc("POST /subscriptions/{id}/actions/{action}", func(w http.ResponseWriter, r *http.Request) {
		id, err := pathID(r)
		if err != nil {
			writeProblem(w, 400, "invalid_id", "subscription id must be a positive integer")
			return
		}
		if err := m.subscriptionAction(r.Context(), id, r.PathValue("action")); err != nil {
			writeError(w, err)
			return
		}
		w.WriteHeader(204)
	})
	mux.HandleFunc("GET /rules", func(w http.ResponseWriter, r *http.Request) {
		items, err := m.repo.Rules(r.Context())
		if err != nil {
			writeError(w, err)
			return
		}
		writeJSON(w, 200, map[string]any{"data": items})
	})
	mux.HandleFunc("POST /rules", func(w http.ResponseWriter, r *http.Request) {
		var input struct {
			Rule string `json:"rule"`
		}
		if json.NewDecoder(r.Body).Decode(&input) != nil {
			writeProblem(w, 400, "invalid_body", "invalid JSON DNS filter rule")
			return
		}
		if err := m.createRule(r.Context(), input.Rule); err != nil {
			writeError(w, err)
			return
		}
		w.WriteHeader(204)
	})
	mux.HandleFunc("POST /rules/{id}/actions/{action}", func(w http.ResponseWriter, r *http.Request) {
		id, err := pathID(r)
		if err != nil {
			writeProblem(w, 400, "invalid_id", "rule id must be a positive integer")
			return
		}
		if err := m.ruleAction(r.Context(), id, r.PathValue("action")); err != nil {
			writeError(w, err)
			return
		}
		w.WriteHeader(204)
	})
	mux.HandleFunc("POST /zones", func(w http.ResponseWriter, r *http.Request) {
		var input struct {
			Name        string `json:"name"`
			Description string `json:"description"`
			Enabled     *bool  `json:"enabled"`
		}
		if json.NewDecoder(r.Body).Decode(&input) != nil || input.Name == "" {
			writeProblem(w, 400, "invalid_zone", "zone name is required")
			return
		}
		enabled := true
		if input.Enabled != nil {
			enabled = *input.Enabled
		}
		item, err := m.createZone(r.Context(), input.Name, input.Description, enabled)
		if err != nil {
			writeError(w, err)
			return
		}
		writeJSON(w, 201, map[string]any{"data": item})
	})
	mux.HandleFunc("GET /zones/{id}", func(w http.ResponseWriter, r *http.Request) {
		id, err := pathID(r)
		if err != nil {
			writeProblem(w, 400, "invalid_id", "zone id must be a positive integer")
			return
		}
		item, err := m.repo.Zone(r.Context(), id)
		if err != nil {
			writeError(w, err)
			return
		}
		writeJSON(w, 200, map[string]any{"data": item})
	})
	mux.HandleFunc("PUT /zones/{id}", func(w http.ResponseWriter, r *http.Request) {
		id, err := pathID(r)
		if err != nil {
			writeProblem(w, 400, "invalid_id", "zone id must be a positive integer")
			return
		}
		var input struct {
			Name        string `json:"name"`
			Description string `json:"description"`
			Enabled     bool   `json:"enabled"`
		}
		if json.NewDecoder(r.Body).Decode(&input) != nil {
			writeProblem(w, 400, "invalid_body", "invalid JSON zone object")
			return
		}
		if err := m.updateZone(r.Context(), id, input.Name, input.Description, input.Enabled); err != nil {
			writeError(w, err)
			return
		}
		w.WriteHeader(204)
	})
	mux.HandleFunc("DELETE /zones/{id}", func(w http.ResponseWriter, r *http.Request) {
		id, err := pathID(r)
		if err != nil {
			writeProblem(w, 400, "invalid_id", "zone id must be a positive integer")
			return
		}
		if err := m.deleteZone(r.Context(), id); err != nil {
			writeError(w, err)
			return
		}
		w.WriteHeader(204)
	})
	mux.HandleFunc("GET /zones/{id}/records", func(w http.ResponseWriter, r *http.Request) {
		id, err := pathID(r)
		if err != nil {
			writeProblem(w, 400, "invalid_id", "zone id must be a positive integer")
			return
		}
		items, err := m.repo.RecordsByZone(r.Context(), id)
		if err != nil {
			writeError(w, err)
			return
		}
		writeJSON(w, 200, map[string]any{"data": items})
	})
	mux.HandleFunc("POST /zones/{id}/records", func(w http.ResponseWriter, r *http.Request) {
		id, err := pathID(r)
		if err != nil {
			writeProblem(w, 400, "invalid_id", "zone id must be a positive integer")
			return
		}
		var item dnsmodule.LocalRecord
		if json.NewDecoder(r.Body).Decode(&item) != nil {
			writeProblem(w, 400, "invalid_body", "invalid JSON DNS record")
			return
		}
		item.ZoneID = id
		if err := m.saveRecord(r.Context(), &item); err != nil {
			writeError(w, err)
			return
		}
		writeJSON(w, 201, map[string]any{"data": item})
	})
	mux.HandleFunc("PUT /records/{id}", func(w http.ResponseWriter, r *http.Request) {
		id, err := pathID(r)
		if err != nil {
			writeProblem(w, 400, "invalid_id", "record id must be a positive integer")
			return
		}
		var item dnsmodule.LocalRecord
		if json.NewDecoder(r.Body).Decode(&item) != nil {
			writeProblem(w, 400, "invalid_body", "invalid JSON DNS record")
			return
		}
		item.ID = id
		if err := m.saveRecord(r.Context(), &item); err != nil {
			writeError(w, err)
			return
		}
		w.WriteHeader(204)
	})
	mux.HandleFunc("DELETE /records/{id}", func(w http.ResponseWriter, r *http.Request) {
		id, err := pathID(r)
		if err != nil {
			writeProblem(w, 400, "invalid_id", "record id must be a positive integer")
			return
		}
		if err := m.deleteRecord(r.Context(), id); err != nil {
			writeError(w, err)
			return
		}
		w.WriteHeader(204)
	})
	return wrapModuleWeb(mux)
}
func summary(p dnsmodule.Provider) dnsmodule.ProviderSummary {
	return dnsmodule.ProviderSummary{ID: p.ID, Name: p.Name, Type: p.Type, CreatedAt: p.CreatedAt, UpdatedAt: p.UpdatedAt}
}
func pathID(r *http.Request) (int64, error) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil || id < 1 {
		return 0, errInvalidID
	}
	return id, nil
}

var errInvalidID = errors.New("invalid ID")

func writeError(w http.ResponseWriter, err error) { writeProblem(w, 500, "dns_error", err.Error()) }
func writeProblem(w http.ResponseWriter, status int, code, message string) {
	writeJSON(w, status, map[string]any{"error": map[string]string{"code": code, "message": message}})
}
func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

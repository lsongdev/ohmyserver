// Package web exposes the ACME module's HTTP API and embedded browser assets.
package web

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"

	acme "github.com/lsongdev/ohmyserver/modules/acme"
)

type Certificate = acme.Certificate
type Provider = acme.Provider
type Account = acme.Account
type Order = acme.Order
type Issuance = acme.Issuance
type IssuanceRequest = acme.IssuanceRequest
type RenewalRequest = acme.RenewalRequest
type DNSChallengeTarget = acme.DNSChallengeTarget
type DNSChallenge = acme.DNSChallenge

type Service = acme.Service

type Module struct{ service Service }

func New(service Service) *Module { return &Module{service: service} }
func (*Module) Name() string      { return "acme" }

func (m *Module) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /certificates", m.listCertificates)
	mux.HandleFunc("GET /certificates/{id}", m.getCertificate)
	mux.HandleFunc("GET /certificates/{id}/download", m.downloadCertificate)
	mux.HandleFunc("POST /certificates/{id}/renew", m.renewCertificate)
	mux.HandleFunc("GET /providers", m.listProviders)
	mux.HandleFunc("GET /accounts", m.listAccounts)
	mux.HandleFunc("GET /orders", m.listOrders)
	mux.HandleFunc("GET /issuances", m.listIssuances)
	mux.HandleFunc("GET /issuances/{id}", m.getIssuance)
	mux.HandleFunc("GET /challenge-targets", m.listChallengeTargets)
	mux.HandleFunc("POST /issuances", m.startIssuance)
	mux.HandleFunc("POST /certificates/csr", m.generateCSR)
	mux.HandleFunc("POST /providers", m.createProvider)
	mux.HandleFunc("DELETE /providers/{id}", m.deleteProvider)
	mux.HandleFunc("POST /providers/{id}/accounts", m.registerAccount)
	mux.HandleFunc("DELETE /accounts/{id}", m.deleteAccount)
	return wrapModuleWeb(mux)
}

func (m *Module) getCertificate(w http.ResponseWriter, r *http.Request) {
	id, err := validID(r.PathValue("id"))
	if err != nil {
		writeError(w, 400, "invalid_id", err.Error())
		return
	}
	item, err := m.service.GetCertificateDetails(r.Context(), id)
	if err != nil {
		writeError(w, 404, "certificate_not_found", err.Error())
		return
	}
	writeData(w, item)
}

func (m *Module) createProvider(w http.ResponseWriter, r *http.Request) {
	var input struct {
		Name         string `json:"name"`
		DirectoryURL string `json:"directory_url"`
	}
	if err := decodeJSON(w, r, &input); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_request", err.Error())
		return
	}
	if input.Name == "" || input.DirectoryURL == "" {
		writeError(w, http.StatusUnprocessableEntity, "invalid_provider", "name and directory_url are required")
		return
	}
	id, err := m.service.CreateProvider(r.Context(), input.Name, input.DirectoryURL)
	if err != nil {
		writeError(w, http.StatusConflict, "provider_create_failed", err.Error())
		return
	}
	location, _, _ := strings.Cut(r.RequestURI, "?")
	w.Header().Set("Location", location)
	w.WriteHeader(http.StatusCreated)
	_ = json.NewEncoder(w).Encode(map[string]any{"data": map[string]int64{"id": id}})
}

func (m *Module) generateCSR(w http.ResponseWriter, r *http.Request) {
	var input struct {
		Domains string `json:"domains"`
	}
	if err := decodeJSON(w, r, &input); err != nil {
		writeError(w, 400, "invalid_request", err.Error())
		return
	}
	item, err := m.service.GenerateCSR(r.Context(), input.Domains)
	if err != nil {
		writeError(w, 422, "csr_rejected", err.Error())
		return
	}
	writeData(w, map[string]any{"id": item.ID, "domains": item.Domains, "csr_pem": item.CSRPEM})
}

func (m *Module) downloadCertificate(w http.ResponseWriter, r *http.Request) {
	id, err := validID(r.PathValue("id"))
	if err != nil {
		writeError(w, 400, "invalid_id", err.Error())
		return
	}
	archive, err := m.service.CertificateArchive(r.Context(), id)
	if err != nil {
		writeError(w, 404, "certificate_not_found", err.Error())
		return
	}
	w.Header().Set("Content-Type", "application/zip")
	w.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=\"certificate-%d.zip\"", id))
	_, _ = w.Write(archive)
}

func (m *Module) renewCertificate(w http.ResponseWriter, r *http.Request) {
	id, err := validID(r.PathValue("id"))
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid_id", err.Error())
		return
	}
	var request RenewalRequest
	if err := decodeJSON(w, r, &request); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_request", err.Error())
		return
	}
	item, err := m.service.RenewCertificate(r.Context(), id, request)
	if err != nil {
		writeError(w, http.StatusUnprocessableEntity, "renewal_rejected", err.Error())
		return
	}
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(http.StatusAccepted)
	_ = json.NewEncoder(w).Encode(map[string]any{"data": item})
}

func (m *Module) deleteProvider(w http.ResponseWriter, r *http.Request) {
	id, err := validID(r.PathValue("id"))
	if err != nil {
		writeError(w, 400, "invalid_id", err.Error())
		return
	}
	if err := m.service.DeleteProvider(r.Context(), id); err != nil {
		writeError(w, 409, "provider_delete_failed", err.Error())
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (m *Module) registerAccount(w http.ResponseWriter, r *http.Request) {
	id, err := validID(r.PathValue("id"))
	if err != nil {
		writeError(w, 400, "invalid_id", err.Error())
		return
	}
	var input struct {
		Email       string `json:"email"`
		AgreedTerms bool   `json:"agreed_terms"`
	}
	if err := decodeJSON(w, r, &input); err != nil {
		writeError(w, 400, "invalid_request", err.Error())
		return
	}
	item, err := m.service.RegisterAccount(r.Context(), id, input.Email, input.AgreedTerms)
	if err != nil {
		writeError(w, 422, "account_registration_failed", err.Error())
		return
	}
	writeData(w, item)
}

func (m *Module) deleteAccount(w http.ResponseWriter, r *http.Request) {
	id, err := validID(r.PathValue("id"))
	if err != nil {
		writeError(w, 400, "invalid_id", err.Error())
		return
	}
	if err := m.service.DeleteAccount(r.Context(), id); err != nil {
		writeError(w, 409, "account_delete_failed", err.Error())
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func validID(value string) (int64, error) {
	id, err := strconv.ParseInt(value, 10, 64)
	if err != nil || id < 1 {
		return 0, fmt.Errorf("invalid id")
	}
	return id, nil
}

func decodeJSON(w http.ResponseWriter, r *http.Request, dst any) error {
	if r.Header.Get("Content-Type") != "application/json" {
		return fmt.Errorf("Content-Type must be application/json")
	}
	defer r.Body.Close()
	decoder := json.NewDecoder(http.MaxBytesReader(w, r.Body, 16<<10))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(dst); err != nil {
		return err
	}
	if err := decoder.Decode(new(any)); err != io.EOF {
		return fmt.Errorf("request must contain one JSON value")
	}
	return nil
}

func (m *Module) listIssuances(w http.ResponseWriter, r *http.Request) {
	items, err := m.service.ListIssuances(r.Context())
	if err != nil {
		writeError(w, http.StatusServiceUnavailable, "acme_unavailable", err.Error())
		return
	}
	if items == nil {
		items = []Issuance{}
	}
	writeData(w, items)
}

func (m *Module) getIssuance(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil || id < 1 {
		writeError(w, http.StatusBadRequest, "invalid_id", "invalid issuance id")
		return
	}
	item, err := m.service.GetIssuance(r.Context(), id)
	if err != nil {
		writeError(w, http.StatusNotFound, "issuance_not_found", err.Error())
		return
	}
	writeData(w, item)
}

func (m *Module) listChallengeTargets(w http.ResponseWriter, r *http.Request) {
	targets, warnings, err := m.service.ListDNSChallengeTargets(r.Context())
	if err != nil {
		writeError(w, http.StatusServiceUnavailable, "dns_unavailable", err.Error())
		return
	}
	if targets == nil {
		targets = []DNSChallengeTarget{}
	}
	writeData(w, map[string]any{"targets": targets, "warnings": warnings})
}

func (m *Module) startIssuance(w http.ResponseWriter, r *http.Request) {
	defer r.Body.Close()
	var request IssuanceRequest
	decoder := json.NewDecoder(http.MaxBytesReader(w, r.Body, 16<<10))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&request); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_request", "invalid issuance request")
		return
	}
	item, err := m.service.StartIssuance(r.Context(), request)
	if err != nil {
		writeError(w, http.StatusUnprocessableEntity, "issuance_rejected", err.Error())
		return
	}
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(http.StatusAccepted)
	_ = json.NewEncoder(w).Encode(map[string]any{"data": item})
}

func writeData(w http.ResponseWriter, value any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	_ = json.NewEncoder(w).Encode(map[string]any{"data": value})
}

func (m *Module) listOrders(w http.ResponseWriter, r *http.Request) {
	items, err := m.service.ListOrders(r.Context())
	if err != nil {
		writeError(w, http.StatusServiceUnavailable, "acme_unavailable", err.Error())
		return
	}
	if items == nil {
		items = []Order{}
	}
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	_ = json.NewEncoder(w).Encode(map[string]any{"data": items})
}

func (m *Module) listProviders(w http.ResponseWriter, r *http.Request) {
	items, err := m.service.ListProviders(r.Context())
	if err != nil {
		writeError(w, http.StatusServiceUnavailable, "acme_unavailable", err.Error())
		return
	}
	if items == nil {
		items = []Provider{}
	}
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	_ = json.NewEncoder(w).Encode(map[string]any{"data": items})
}

func (m *Module) listAccounts(w http.ResponseWriter, r *http.Request) {
	items, err := m.service.ListAccounts(r.Context())
	if err != nil {
		writeError(w, http.StatusServiceUnavailable, "acme_unavailable", err.Error())
		return
	}
	if items == nil {
		items = []Account{}
	}
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	_ = json.NewEncoder(w).Encode(map[string]any{"data": items})
}

func (m *Module) listCertificates(w http.ResponseWriter, r *http.Request) {
	items, err := m.service.ListACMECertificates(r.Context())
	if err != nil {
		writeError(w, http.StatusServiceUnavailable, "acme_unavailable", err.Error())
		return
	}
	if items == nil {
		items = []Certificate{}
	}
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	_ = json.NewEncoder(w).Encode(map[string]any{"data": items})
}

func writeError(w http.ResponseWriter, status int, code, message string) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(map[string]any{"error": map[string]string{"code": code, "message": message}})
}

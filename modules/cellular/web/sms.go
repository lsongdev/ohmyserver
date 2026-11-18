package web

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/lsongdev/ohmyserver/modules/cellular/internal/modem"
)

func (m *Cellular) requireSMS(handler http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if !m.authenticated {
			commandError(w, http.StatusForbidden, "Enable panel sign-in to access SMS.", nil)
			return
		}
		handler(w, r)
	}
}

func (m *Cellular) listSMS(w http.ResponseWriter, r *http.Request) {
	inbox, err := m.monitor.ListSMS(r.Context())
	if err != nil {
		commandError(w, modemHTTPStatus(err), err.Error(), nil)
		return
	}
	writeJSON(w, http.StatusOK, inbox)
}

func (m *Cellular) sendSMS(w http.ResponseWriter, r *http.Request) {
	var input struct {
		Number string `json:"number"`
		Text   string `json:"text"`
	}
	d := json.NewDecoder(http.MaxBytesReader(w, r.Body, 16<<10))
	d.DisallowUnknownFields()
	if err := d.Decode(&input); err != nil || d.Decode(new(any)) != io.EOF {
		commandError(w, http.StatusBadRequest, "Invalid SMS request.", nil)
		return
	}
	result, err := m.monitor.SendSMS(r.Context(), strings.TrimSpace(input.Number), input.Text)
	if err == nil {
		writeJSON(w, http.StatusOK, result)
		return
	}
	var invalid modem.SMSInputError
	if errors.As(err, &invalid) {
		commandError(w, http.StatusBadRequest, "Check the message fields.", map[string]string{invalid.Field: invalid.Message})
		return
	}
	message := err.Error()
	if result.Uncertain {
		message = fmt.Sprintf("The modem confirmed %d of %d parts. The next part may have been sent. Check before sending again.", len(result.References), result.Parts)
	} else if len(result.References) > 0 {
		message = fmt.Sprintf("The modem confirmed %d of %d parts before an error. Check before sending again.", len(result.References), result.Parts)
	}
	commandError(w, modemHTTPStatus(err), message, nil)
}

func modemHTTPStatus(err error) int {
	if errors.Is(err, modem.ErrOffline) || errors.Is(err, modem.ErrBusy) {
		return http.StatusServiceUnavailable
	}
	return http.StatusBadGateway
}

func commandError(w http.ResponseWriter, status int, message string, fields map[string]string) {
	writeJSON(w, status, map[string]any{"error": map[string]any{"message": message, "fields": fields}})
}

func writeJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}

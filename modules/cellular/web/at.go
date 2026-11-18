package web

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"

	"github.com/lsongdev/ohmyserver/modules/cellular/internal/modem"
)

func (m *Cellular) command(w http.ResponseWriter, r *http.Request) {
	if !m.authenticated {
		commandError(w, http.StatusForbidden, "Enable panel sign-in to send AT commands.", nil)
		return
	}
	var input struct {
		Command string `json:"command"`
	}
	decoder := json.NewDecoder(http.MaxBytesReader(w, r.Body, 2048))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&input); err != nil || decoder.Decode(new(any)) != io.EOF {
		commandError(w, http.StatusBadRequest, "Invalid AT command request.", nil)
		return
	}
	result, err := m.monitor.Command(r.Context(), strings.Trim(input.Command, " "))
	if err != nil {
		var invalid modem.InvalidCommand
		if errors.As(err, &invalid) {
			commandError(w, http.StatusBadRequest, invalid.Error(), map[string]string{"command": invalid.Error()})
			return
		}
		commandError(w, modemHTTPStatus(err), err.Error(), nil)
		return
	}
	writeJSON(w, http.StatusOK, result)
}

package web

import (
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strconv"
)

type Module struct{ repo *Repository }

func New(repo *Repository) *Module { return &Module{repo: repo} }
func (*Module) Name() string       { return "settings" }
func (m *Module) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /entries", m.list)
	mux.HandleFunc("PUT /entries/{id}", m.update)
	mux.HandleFunc("POST /entries", m.create)
	mux.HandleFunc("DELETE /entries/{id}", m.delete)
	return wrapModuleWeb(mux)
}

func (m *Module) list(w http.ResponseWriter, r *http.Request) {
	items, err := m.repo.List(r.Context())
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"data": items})
}
func (m *Module) create(w http.ResponseWriter, r *http.Request) {
	var input struct {
		Name        string `json:"name"`
		Type        string `json:"type"`
		Value       string `json:"value"`
		Description string `json:"description"`
	}
	if err := decodeJSON(w, r, &input); err != nil {
		writeProblem(w, http.StatusBadRequest, "invalid_request", err.Error())
		return
	}
	if input.Name == "" || input.Value == "" || !validType(input.Type) {
		writeProblem(w, http.StatusUnprocessableEntity, "invalid_setting", "name, a supported type and value are required")
		return
	}
	item, err := m.repo.Create(r.Context(), input.Name, input.Type, input.Value, input.Description)
	if err != nil {
		writeError(w, err)
		return
	}
	w.Header().Set("Location", fmt.Sprintf("entries/%d", item.ID))
	writeJSON(w, http.StatusCreated, map[string]any{"data": item})
}
func (m *Module) update(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		writeProblem(w, http.StatusBadRequest, "invalid_id", "setting id must be an integer")
		return
	}
	var input struct {
		Value string `json:"value"`
	}
	if err := decodeJSON(w, r, &input); err != nil {
		writeProblem(w, http.StatusBadRequest, "invalid_body", err.Error())
		return
	}
	if err := m.repo.Update(r.Context(), id, input.Value); err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"data": map[string]bool{"updated": true}})
}

func decodeJSON(w http.ResponseWriter, r *http.Request, value any) error {
	d := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<20))
	d.DisallowUnknownFields()
	if err := d.Decode(value); err != nil {
		return err
	}
	var extra any
	if err := d.Decode(&extra); err != io.EOF {
		if err == nil {
			return errors.New("request body must contain one JSON value")
		}
		return err
	}
	return nil
}

func (m *Module) delete(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		writeProblem(w, http.StatusBadRequest, "invalid_id", "setting id must be an integer")
		return
	}
	if err := m.repo.Delete(r.Context(), id); err != nil {
		writeError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
func writeError(w http.ResponseWriter, err error) {
	if err == sql.ErrNoRows {
		writeProblem(w, http.StatusNotFound, "not_found", "setting not found")
		return
	}
	writeProblem(w, http.StatusInternalServerError, "settings_error", err.Error())
}
func writeProblem(w http.ResponseWriter, status int, code, message string) {
	writeJSON(w, status, map[string]any{"error": map[string]string{"code": code, "message": message}})
}
func writeJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}

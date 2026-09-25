package web

import (
	"encoding/json"
	"errors"
	"mime"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	files "github.com/lsongdev/ohmyserver/modules/files"
)

const maxUploadBytes = 64 << 20

type Service = files.Service

type Module struct{ service Service }

func New(service Service) *Module { return &Module{service: service} }
func (*Module) Name() string      { return "files" }
func (m *Module) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /entries", func(w http.ResponseWriter, r *http.Request) {
		listing, err := m.service.List(r.Context(), r.URL.Query().Get("path"), r.URL.Query().Get("show_hidden") == "true")
		if err != nil {
			fileError(w, err)
			return
		}
		fileJSON(w, 200, map[string]any{"data": listing})
	})
	mux.HandleFunc("POST /entries", func(w http.ResponseWriter, r *http.Request) {
		var body struct {
			Action  string `json:"action"`
			Path    string `json:"path"`
			Name    string `json:"name"`
			Content string `json:"content"`
		}
		r.Body = http.MaxBytesReader(w, r.Body, 1<<20)
		if json.NewDecoder(r.Body).Decode(&body) != nil {
			fileProblem(w, 400, "invalid_body", "invalid JSON request")
			return
		}
		var err error
		switch body.Action {
		case "create_file":
			err = m.service.CreateFile(r.Context(), body.Path, body.Name, body.Content)
		case "create_folder":
			err = m.service.CreateFolder(r.Context(), body.Path, body.Name)
		default:
			fileProblem(w, 400, "invalid_action", "action must be create_file or create_folder")
			return
		}
		if err != nil {
			fileError(w, err)
			return
		}
		w.WriteHeader(http.StatusCreated)
	})
	mux.HandleFunc("POST /upload", func(w http.ResponseWriter, r *http.Request) {
		r.Body = http.MaxBytesReader(w, r.Body, maxUploadBytes)
		if err := r.ParseMultipartForm(maxUploadBytes); err != nil {
			fileProblem(w, 400, "invalid_upload", "upload must be a multipart form smaller than 64 MiB")
			return
		}
		defer r.MultipartForm.RemoveAll()
		source, header, err := r.FormFile("file")
		if err != nil {
			fileProblem(w, 400, "missing_file", "file field is required")
			return
		}
		defer source.Close()
		name := filepath.Base(strings.ReplaceAll(header.Filename, "\\", "/"))
		if err = m.service.Upload(r.Context(), r.FormValue("path"), name, source); err != nil {
			fileError(w, err)
			return
		}
		w.WriteHeader(http.StatusCreated)
	})
	mux.HandleFunc("DELETE /entries", func(w http.ResponseWriter, r *http.Request) {
		if err := m.service.Delete(r.Context(), r.URL.Query().Get("path")); err != nil {
			fileError(w, err)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	})
	mux.HandleFunc("GET /download", func(w http.ResponseWriter, r *http.Request) {
		file, info, err := m.service.Open(r.Context(), r.URL.Query().Get("path"))
		if err != nil {
			fileError(w, err)
			return
		}
		defer file.Close()
		disposition := mime.FormatMediaType("attachment", map[string]string{"filename": info.Name()})
		w.Header().Set("Content-Disposition", disposition)
		http.ServeContent(w, r, info.Name(), info.ModTime(), file)
	})
	return wrapModuleWeb(mux)
}
func fileJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}
func fileProblem(w http.ResponseWriter, status int, code, message string) {
	fileJSON(w, status, map[string]any{"error": map[string]string{"code": code, "message": message}})
}
func fileError(w http.ResponseWriter, err error) {
	status := http.StatusInternalServerError
	switch {
	case errors.Is(err, files.ErrInvalidPath), errors.Is(err, files.ErrInvalidName):
		status = http.StatusBadRequest
	case errors.Is(err, os.ErrNotExist):
		status = http.StatusNotFound
	case errors.Is(err, os.ErrPermission):
		status = http.StatusForbidden
	}
	fileProblem(w, status, "filesystem_error", err.Error())
}

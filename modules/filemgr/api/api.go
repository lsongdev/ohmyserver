// Package api is the file manager's JSON API. Uploads are multipart and
// stream to disk; downloads serve files with ranges and directories as
// tar.gz archives built on the fly.
package api

import (
	"archive/tar"
	"compress/gzip"
	"encoding/json"
	"errors"
	"io"
	"io/fs"
	"log/slog"
	"mime"
	"net/http"
	"os"
	"path"
	"path/filepath"

	"github.com/lsongdev/ohmyserver/modules/filemgr/files"
)

// Options configures the API.
type Options struct {
	Logger *slog.Logger
	// Root is the directory the file manager shows as "/"; "/" by default.
	Root string
}

// Service serves the API.
type Service struct {
	fs     *files.FS
	logger *slog.Logger
}

// New returns the API.
func New(opts Options) (*Service, error) {
	root := opts.Root
	if root == "" {
		root = "/"
	}
	f, err := files.New(root)
	if err != nil {
		return nil, err
	}
	logger := opts.Logger
	if logger == nil {
		logger = slog.Default()
	}
	return &Service{fs: f, logger: logger}, nil
}

// Listing is a directory and what is in it.
type Listing struct {
	Dir     files.Entry   `json:"dir"`
	Entries []files.Entry `json:"entries"`
}

type pathsRequest struct {
	Paths []string `json:"paths"`
	To    string   `json:"to"` // the destination directory, for move and copy
}

// Handler serves the API. Successful responses are {"data": ...}; failures
// are {"error": {"code": ..., "message": ...}} with a matching status.
func (s *Service) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /list", func(w http.ResponseWriter, r *http.Request) {
		p := r.URL.Query().Get("path")
		dir, err := s.fs.Stat(p)
		if err != nil {
			respond(w, nil, err)
			return
		}
		entries, err := s.fs.List(p)
		respond(w, Listing{Dir: dir, Entries: entries}, err)
	})
	mux.HandleFunc("GET /text", func(w http.ResponseWriter, r *http.Request) {
		t, err := s.fs.Read(r.URL.Query().Get("path"))
		respond(w, t, err)
	})
	mux.HandleFunc("PUT /text", func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			Path   string `json:"path"`
			Text   string `json:"text"`
			Create bool   `json:"create"`
		}
		if !decode(w, r, &req, files.MaxText+4096) {
			return
		}
		err := s.fs.Write(req.Path, req.Text, req.Create)
		s.log(err, "save", "path", req.Path)
		respond(w, map[string]string{"path": req.Path}, err)
	})
	mux.HandleFunc("GET /download", s.download)
	mux.HandleFunc("POST /upload", s.upload)
	mux.HandleFunc("POST /mkdir", func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			Path string `json:"path"`
		}
		if decode(w, r, &req, 4096) {
			err := s.fs.Mkdir(req.Path)
			s.log(err, "mkdir", "path", req.Path)
			respond(w, map[string]string{"path": req.Path}, err)
		}
	})
	mux.HandleFunc("POST /rename", func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			Path string `json:"path"`
			Name string `json:"name"`
		}
		if decode(w, r, &req, 4096) {
			e, err := s.fs.Rename(req.Path, req.Name)
			s.log(err, "rename", "path", req.Path, "name", req.Name)
			respond(w, e, err)
		}
	})
	for action, run := range map[string]func(pathsRequest) error{
		"delete": func(req pathsRequest) error { return s.fs.Delete(req.Paths) },
		"move":   func(req pathsRequest) error { return s.fs.Move(req.Paths, req.To) },
		"copy":   func(req pathsRequest) error { return s.fs.Copy(req.Paths, req.To) },
	} {
		mux.HandleFunc("POST /"+action, func(w http.ResponseWriter, r *http.Request) {
			var req pathsRequest
			if decode(w, r, &req, 1<<20) {
				err := run(req)
				s.log(err, action, "paths", req.Paths, "to", req.To)
				respond(w, map[string]int{"count": len(req.Paths)}, err)
			}
		})
	}
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		writeError(w, http.StatusNotFound, "not_found", "no such endpoint")
	})
	return mux
}

// log records changes, which an operator may want to trace later.
func (s *Service) log(err error, action string, args ...any) {
	if err == nil {
		s.logger.Info("files: "+action, args...)
	}
}

// upload saves the files of a multipart form in ?dir=, streaming each part
// to disk. ?overwrite=1 replaces existing files.
func (s *Service) upload(w http.ResponseWriter, r *http.Request) {
	dir := r.URL.Query().Get("dir")
	overwrite := r.URL.Query().Get("overwrite") == "1"
	reader, err := r.MultipartReader()
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid_request", "expected a multipart upload: "+err.Error())
		return
	}
	saved := []files.Entry{}
	for {
		part, err := reader.NextPart()
		if err == io.EOF {
			break
		}
		if err != nil {
			respond(w, nil, err)
			return
		}
		if part.FileName() == "" {
			continue
		}
		// Browsers send only the base name; never trust a path in it.
		e, err := s.fs.Upload(dir, path.Base(filepath.ToSlash(part.FileName())), part, overwrite)
		part.Close()
		if err != nil {
			respond(w, nil, err)
			return
		}
		s.logger.Info("files: upload", "path", e.Path, "size", e.Size)
		saved = append(saved, e)
	}
	respond(w, saved, nil)
}

// download serves ?path= as an attachment, or inline with ?inline=1 (for
// previews). A directory downloads as a tar.gz archive.
func (s *Service) download(w http.ResponseWriter, r *http.Request) {
	p := r.URL.Query().Get("path")
	file, info, err := s.fs.Open(p)
	if err != nil {
		respond(w, nil, err)
		return
	}
	defer file.Close()
	name := info.Name()
	if info.IsDir() {
		s.archive(w, file.Name(), name)
		return
	}
	if !info.Mode().IsRegular() {
		respond(w, nil, errors.Join(files.ErrInvalid, errors.New(p+" is not a file")))
		return
	}
	disposition := "attachment"
	if r.URL.Query().Get("inline") == "1" {
		disposition = "inline"
		// Never let an uploaded HTML or SVG file run as a page of the panel.
		w.Header().Set("Content-Security-Policy", "default-src 'none'; img-src 'self' data:; media-src 'self'; style-src 'unsafe-inline'; sandbox")
	}
	w.Header().Set("Content-Disposition", mime.FormatMediaType(disposition, map[string]string{"filename": name}))
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.Header().Set("Cache-Control", "no-store")
	http.ServeContent(w, r, name, info.ModTime(), file)
}

// archive streams a directory as tar.gz. Errors after the first byte can
// only cut the archive short, which the client sees as a broken download.
func (s *Service) archive(w http.ResponseWriter, root, name string) {
	w.Header().Set("Content-Type", "application/gzip")
	w.Header().Set("Content-Disposition", mime.FormatMediaType("attachment", map[string]string{"filename": name + ".tar.gz"}))
	w.Header().Set("Cache-Control", "no-store")
	gz := gzip.NewWriter(w)
	tw := tar.NewWriter(gz)
	err := filepath.WalkDir(root, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		info, err := d.Info()
		if err != nil {
			return err
		}
		link := ""
		if info.Mode()&fs.ModeSymlink != 0 {
			link, _ = os.Readlink(p)
		}
		hdr, err := tar.FileInfoHeader(info, link)
		if err != nil {
			return nil // sockets and the like are left out
		}
		rel, _ := filepath.Rel(filepath.Dir(root), p)
		hdr.Name = filepath.ToSlash(rel)
		if info.IsDir() {
			hdr.Name += "/"
		}
		if err := tw.WriteHeader(hdr); err != nil {
			return err
		}
		if !info.Mode().IsRegular() {
			return nil
		}
		f, err := os.Open(p)
		if err != nil {
			return err
		}
		defer f.Close()
		_, err = io.Copy(tw, f)
		return err
	})
	if err == nil {
		err = tw.Close()
	}
	if err == nil {
		err = gz.Close()
	}
	if err != nil {
		s.logger.Warn("files: archive cut short", "dir", root, "err", err)
	}
}

func decode(w http.ResponseWriter, r *http.Request, v any, limit int64) bool {
	dec := json.NewDecoder(http.MaxBytesReader(w, r.Body, limit))
	dec.DisallowUnknownFields()
	if err := dec.Decode(v); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_request", "invalid JSON: "+err.Error())
		return false
	}
	return true
}

func respond(w http.ResponseWriter, data any, err error) {
	switch {
	case err == nil:
		writeJSON(w, http.StatusOK, map[string]any{"data": data})
	case errors.Is(err, files.ErrInvalid):
		writeError(w, http.StatusBadRequest, "invalid_request", err.Error())
	case errors.Is(err, files.ErrNotFound):
		writeError(w, http.StatusNotFound, "not_found", err.Error())
	case errors.Is(err, files.ErrExists):
		writeError(w, http.StatusConflict, "exists", err.Error())
	case errors.Is(err, files.ErrProtected):
		writeError(w, http.StatusForbidden, "protected", err.Error())
	default:
		writeError(w, http.StatusInternalServerError, "failed", err.Error())
	}
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

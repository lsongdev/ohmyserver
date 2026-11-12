package api

import (
	"fmt"
	"net/http"

	"github.com/lsongdev/ohmyserver/modules/pkgmgr/system"
)

type repositoryRequest struct {
	ID       string `json:"id"`
	Content  string `json:"content"`
	Revision string `json:"revision"`
}

func (s *Service) registerRepositories(mux *http.ServeMux) {
	mux.HandleFunc("GET /repositories", func(w http.ResponseWriter, r *http.Request) {
		files, err := s.repositories.List()
		respond(w, http.StatusOK, nonNil(files), err)
	})
	mux.HandleFunc("GET /repositories/{id}", func(w http.ResponseWriter, r *http.Request) {
		file, err := s.repositories.Read(r.PathValue("id"))
		respond(w, http.StatusOK, file, err)
	})
	mux.HandleFunc("POST /repositories", func(w http.ResponseWriter, r *http.Request) {
		var req repositoryRequest
		if decodeLimit(w, r, &req, 1<<20) {
			s.changeRepository(w, func() error { return s.repositories.Write(req.ID, req.Content, "") })
		}
	})
	mux.HandleFunc("PUT /repositories/{id}", func(w http.ResponseWriter, r *http.Request) {
		var req repositoryRequest
		if !decodeLimit(w, r, &req, 1<<20) {
			return
		}
		if req.Revision == "" {
			respond(w, 0, nil, fmt.Errorf("%w: revision is required", system.ErrInvalid))
			return
		}
		s.changeRepository(w, func() error { return s.repositories.Write(r.PathValue("id"), req.Content, req.Revision) })
	})
	mux.HandleFunc("DELETE /repositories/{id}", func(w http.ResponseWriter, r *http.Request) {
		s.changeRepository(w, func() error { return s.repositories.Delete(r.PathValue("id"), r.URL.Query().Get("revision")) })
	})
}

// Serialize source changes with package jobs and other source changes.
func (s *Service) changeRepository(w http.ResponseWriter, change func() error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.running != nil {
		respond(w, 0, nil, ErrBusy)
		return
	}
	respond(w, http.StatusNoContent, nil, change())
}

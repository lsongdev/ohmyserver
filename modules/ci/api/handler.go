package api

import (
	"crypto/hmac"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strconv"
	"strings"

	"github.com/lsongdev/ohmyserver/modules/ci/store"
)

// Handler serves the API. Successful responses are {"data": ...}; failures
// are {"error": {"code": ..., "message": ...}} with a matching status.
//
// POST /hooks/{repo} takes push webhooks from GitHub, Gitea and GitLab and
// authenticates them with the repository's webhook secret; a panel serves
// it without its own sign-in (the manifest lists it as public).
func (s *Service) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /repos", func(w http.ResponseWriter, r *http.Request) {
		repos, err := s.Repos(r.Context())
		respond(w, http.StatusOK, repos, err)
	})
	mux.HandleFunc("POST /repos", func(w http.ResponseWriter, r *http.Request) {
		var in RepoInput
		if decode(w, r, &in) {
			v, err := s.CreateRepo(r.Context(), in)
			respond(w, http.StatusCreated, v, err)
		}
	})
	mux.HandleFunc("GET /repos/{id}", func(w http.ResponseWriter, r *http.Request) {
		if id, ok := pathID(w, r, "id"); ok {
			v, err := s.Repo(r.Context(), id)
			respond(w, http.StatusOK, v, err)
		}
	})
	mux.HandleFunc("PUT /repos/{id}", func(w http.ResponseWriter, r *http.Request) {
		var in RepoInput
		if id, ok := pathID(w, r, "id"); ok && decode(w, r, &in) {
			v, err := s.UpdateRepo(r.Context(), id, in)
			respond(w, http.StatusOK, v, err)
		}
	})
	mux.HandleFunc("DELETE /repos/{id}", func(w http.ResponseWriter, r *http.Request) {
		if id, ok := pathID(w, r, "id"); ok {
			respond(w, http.StatusNoContent, nil, s.DeleteRepo(r.Context(), id))
		}
	})
	mux.HandleFunc("POST /repos/{id}/webhook-secret", func(w http.ResponseWriter, r *http.Request) {
		if id, ok := pathID(w, r, "id"); ok {
			v, err := s.RotateWebhookSecret(r.Context(), id)
			respond(w, http.StatusOK, v, err)
		}
	})
	mux.HandleFunc("GET /repos/{id}/builds", func(w http.ResponseWriter, r *http.Request) {
		if id, ok := pathID(w, r, "id"); ok {
			builds, err := s.Builds(r.Context(), id)
			respond(w, http.StatusOK, builds, err)
		}
	})
	mux.HandleFunc("POST /repos/{id}/builds", func(w http.ResponseWriter, r *http.Request) {
		var in struct {
			Branch string `json:"branch"`
		}
		if r.ContentLength != 0 && !decode(w, r, &in) {
			return
		}
		if id, ok := pathID(w, r, "id"); ok {
			b, err := s.Trigger(r.Context(), id, "manual", strings.TrimSpace(in.Branch), "", "", "")
			respond(w, http.StatusCreated, b, err)
		}
	})
	mux.HandleFunc("GET /builds", func(w http.ResponseWriter, r *http.Request) {
		builds, err := s.Builds(r.Context(), 0)
		respond(w, http.StatusOK, builds, err)
	})
	mux.HandleFunc("GET /builds/{id}", func(w http.ResponseWriter, r *http.Request) {
		if id, ok := pathID(w, r, "id"); ok {
			b, err := s.Build(r.Context(), id)
			respond(w, http.StatusOK, b, err)
		}
	})
	mux.HandleFunc("GET /builds/{id}/steps/{pos}/log", func(w http.ResponseWriter, r *http.Request) {
		id, ok := pathID(w, r, "id")
		if !ok {
			return
		}
		pos, err := strconv.Atoi(r.PathValue("pos"))
		offset, _ := strconv.ParseInt(r.URL.Query().Get("offset"), 10, 64)
		if err != nil || offset < 0 {
			writeError(w, http.StatusBadRequest, "invalid_request", "invalid step or offset")
			return
		}
		log, err := s.StepLog(r.Context(), id, pos, offset)
		respond(w, http.StatusOK, log, err)
	})
	mux.HandleFunc("POST /builds/{id}/cancel", func(w http.ResponseWriter, r *http.Request) {
		if id, ok := pathID(w, r, "id"); ok {
			respond(w, http.StatusAccepted, nil, s.Cancel(r.Context(), id))
		}
	})
	mux.HandleFunc("POST /builds/{id}/restart", func(w http.ResponseWriter, r *http.Request) {
		if id, ok := pathID(w, r, "id"); ok {
			b, err := s.Restart(r.Context(), id)
			respond(w, http.StatusCreated, b, err)
		}
	})
	mux.HandleFunc("POST /hooks/{id}", s.webhook)
	mux.HandleFunc("GET /summary", func(w http.ResponseWriter, r *http.Request) {
		sum, err := s.Summary(r.Context())
		respond(w, http.StatusOK, sum, err)
	})
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		writeError(w, http.StatusNotFound, "not_found", "no such API endpoint")
	})
	return mux
}

// push is the part of a push event the build needs; GitHub and Gitea share
// a shape, GitLab differs.
type push struct {
	Ref        string `json:"ref"`
	After      string `json:"after"`
	HeadCommit *struct {
		Message string `json:"message"`
		Author  struct {
			Name string `json:"name"`
		} `json:"author"`
	} `json:"head_commit"`
	CheckoutSHA string `json:"checkout_sha"` // GitLab
	UserName    string `json:"user_name"`    // GitLab
	Commits     []struct {
		ID      string `json:"id"`
		Message string `json:"message"`
	} `json:"commits"`
}

// webhook queues a build for a push to a branch. The request is
// authenticated with the repository's webhook secret: an HMAC-SHA256
// signature (GitHub, Gitea) or the secret itself as a token (GitLab).
func (s *Service) webhook(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r, "id")
	if !ok {
		return
	}
	repo, err := s.store.Repo(r.Context(), id)
	if err != nil {
		writeError(w, http.StatusNotFound, "not_found", "unknown repository") // also for a forged id
		return
	}
	body, err := io.ReadAll(http.MaxBytesReader(w, r.Body, 10<<20))
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid_request", "body too large")
		return
	}
	if !authentic(r, body, repo.WebhookSecret) {
		writeError(w, http.StatusUnauthorized, "unauthorized", "bad webhook signature")
		return
	}
	event := r.Header.Get("X-GitHub-Event") + r.Header.Get("X-Gitea-Event") + r.Header.Get("X-Gitlab-Event")
	if event == "ping" {
		writeJSON(w, http.StatusOK, map[string]any{"data": "pong"})
		return
	}
	if event != "push" && event != "Push Hook" {
		writeJSON(w, http.StatusAccepted, map[string]any{"data": "ignored: " + event})
		return
	}
	var p push
	if err := json.Unmarshal(body, &p); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_request", "invalid push payload")
		return
	}
	branch, isBranch := strings.CutPrefix(p.Ref, "refs/heads/")
	sha, message, author := p.After, "", p.UserName
	if p.CheckoutSHA != "" {
		sha = p.CheckoutSHA
	}
	if p.HeadCommit != nil {
		message, author = p.HeadCommit.Message, p.HeadCommit.Author.Name
	} else if n := len(p.Commits); n > 0 {
		message = p.Commits[n-1].Message
	}
	if !isBranch || sha == "" || strings.Trim(sha, "0") == "" { // a tag, or a deleted branch
		writeJSON(w, http.StatusAccepted, map[string]any{"data": "ignored: not a branch push"})
		return
	}
	message, _, _ = strings.Cut(message, "\n")
	b, err := s.Trigger(r.Context(), id, "push", branch, sha, message, author)
	respond(w, http.StatusCreated, b, err)
}

func authentic(r *http.Request, body []byte, secret string) bool {
	if token := r.Header.Get("X-Gitlab-Token"); token != "" {
		return subtle.ConstantTimeCompare([]byte(token), []byte(secret)) == 1
	}
	sig := strings.TrimPrefix(r.Header.Get("X-Hub-Signature-256"), "sha256=")
	if sig == "" {
		sig = r.Header.Get("X-Gitea-Signature")
	}
	got, err := hex.DecodeString(sig)
	if err != nil || len(got) == 0 {
		return false
	}
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write(body)
	return hmac.Equal(got, mac.Sum(nil))
}

func respond(w http.ResponseWriter, status int, data any, err error) {
	switch {
	case err == nil:
	case errors.Is(err, ErrInvalid):
		writeError(w, http.StatusUnprocessableEntity, "invalid_request", err.Error())
		return
	case errors.Is(err, store.ErrNotFound):
		writeError(w, http.StatusNotFound, "not_found", err.Error())
		return
	case errors.Is(err, store.ErrConflict):
		writeError(w, http.StatusConflict, "conflict", err.Error())
		return
	default:
		writeError(w, http.StatusInternalServerError, "internal", err.Error())
		return
	}
	if status == http.StatusNoContent || data == nil {
		w.WriteHeader(status)
		return
	}
	writeJSON(w, status, map[string]any{"data": data})
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

func decode(w http.ResponseWriter, r *http.Request, v any) bool {
	dec := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<20))
	dec.DisallowUnknownFields()
	if err := dec.Decode(v); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_request", "invalid JSON body: "+err.Error())
		return false
	}
	return true
}

func pathID(w http.ResponseWriter, r *http.Request, name string) (int64, bool) {
	id, err := strconv.ParseInt(r.PathValue(name), 10, 64)
	if err != nil || id < 1 {
		writeError(w, http.StatusBadRequest, "invalid_request", "invalid id")
		return 0, false
	}
	return id, true
}

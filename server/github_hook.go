package server

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"mime"
	"net/http"
	"strconv"
	"strings"
)

const githubWebhookMaxBody = 2 << 20

type githubPushPayload struct {
	Ref        string `json:"ref"`
	After      string `json:"after"`
	Deleted    bool   `json:"deleted"`
	HeadCommit struct {
		Message string `json:"message"`
		Author  struct {
			Name string `json:"name"`
		} `json:"author"`
	} `json:"head_commit"`
}

func newWebhookSecret() (string, error) {
	secret := make([]byte, 32)
	if _, err := rand.Read(secret); err != nil {
		return "", fmt.Errorf("generate webhook secret: %w", err)
	}
	return hex.EncodeToString(secret), nil
}

func validGitHubSignature(secret string, body []byte, signature string) bool {
	if secret == "" || signature == "" {
		return false
	}
	mac := hmac.New(sha256.New, []byte(secret))
	_, _ = mac.Write(body)
	expected := []byte("sha256=" + hex.EncodeToString(mac.Sum(nil)))
	return hmac.Equal(expected, []byte(signature))
}

func validGitCommitSHA(value string) bool {
	if len(value) != 40 {
		return false
	}
	_, err := hex.DecodeString(value)
	return err == nil
}

func validGitHubBranch(value string) bool {
	return value != "" &&
		!strings.HasPrefix(value, "-") &&
		!strings.HasPrefix(value, "refs/") &&
		!strings.HasPrefix(value, "/") &&
		!strings.HasSuffix(value, "/") &&
		!strings.HasSuffix(value, ".") &&
		!strings.HasSuffix(value, ".lock") &&
		!strings.Contains(value, "..") &&
		!strings.Contains(value, "//") &&
		!strings.Contains(value, "@{") &&
		!strings.ContainsAny(value, " ~^:?*[\\")
}

func writeWebhookJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}

// GitHubWebhookHandler validates GitHub deliveries and starts the same
// deployment pipeline used by the manual button.
func (s *Server) GitHubWebhookHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.Header().Set("Allow", http.MethodPost)
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	mediaType, _, err := mime.ParseMediaType(r.Header.Get("Content-Type"))
	if err != nil || mediaType != "application/json" {
		http.Error(w, "Content-Type must be application/json", http.StatusUnsupportedMediaType)
		return
	}

	projectID, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	project, err := s.GetProjectByID(projectID)
	if err != nil {
		http.NotFound(w, r)
		return
	}

	r.Body = http.MaxBytesReader(w, r.Body, githubWebhookMaxBody)
	body, err := io.ReadAll(r.Body)
	if err != nil {
		var maxBytesErr *http.MaxBytesError
		if errors.As(err, &maxBytesErr) {
			http.Error(w, "Payload too large", http.StatusRequestEntityTooLarge)
			return
		}
		http.Error(w, "Invalid payload", http.StatusBadRequest)
		return
	}
	if !validGitHubSignature(project.WebhookSecret, body, r.Header.Get("X-Hub-Signature-256")) {
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}

	event := r.Header.Get("X-GitHub-Event")
	if event == "ping" {
		writeWebhookJSON(w, http.StatusOK, H{"status": "ok"})
		return
	}
	if event != "push" {
		writeWebhookJSON(w, http.StatusOK, H{"status": "ignored", "reason": "event"})
		return
	}

	deliveryID := strings.TrimSpace(r.Header.Get("X-GitHub-Delivery"))
	if deliveryID == "" {
		http.Error(w, "Missing X-GitHub-Delivery", http.StatusBadRequest)
		return
	}
	var payload githubPushPayload
	if err := json.Unmarshal(body, &payload); err != nil {
		http.Error(w, "Invalid payload", http.StatusBadRequest)
		return
	}
	if payload.Deleted {
		writeWebhookJSON(w, http.StatusOK, H{"status": "ignored", "reason": "deleted"})
		return
	}
	branch := project.WebhookBranch
	if branch == "" {
		branch = "main"
	}
	if payload.Ref != "refs/heads/"+branch {
		writeWebhookJSON(w, http.StatusOK, H{"status": "ignored", "reason": "branch"})
		return
	}
	if !validGitCommitSHA(payload.After) {
		http.Error(w, "Invalid commit SHA", http.StatusBadRequest)
		return
	}

	deployment, duplicate, err := s.StartGitHubDeployment(
		project,
		payload.After,
		payload.HeadCommit.Message,
		payload.HeadCommit.Author.Name,
		deliveryID,
	)
	if err != nil {
		status := http.StatusServiceUnavailable
		if errors.Is(err, ErrDeploymentInProgress) {
			status = http.StatusConflict
		}
		http.Error(w, err.Error(), status)
		return
	}
	if duplicate {
		writeWebhookJSON(w, http.StatusOK, H{
			"status":        "duplicate",
			"deployment_id": deployment.ID,
		})
		return
	}
	writeWebhookJSON(w, http.StatusAccepted, H{
		"status":        "accepted",
		"deployment_id": deployment.ID,
	})
}

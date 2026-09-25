package web

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"mime"
	"net/http"
	"strings"
)

const githubWebhookMaxBody = 2 << 20

type WebhookResponse struct {
	Status       string `json:"status"`
	Reason       string `json:"reason,omitempty"`
	DeploymentID int64  `json:"deployment_id,omitempty"`
}

type webhookError struct {
	status  int
	message string
}

func (e *webhookError) Error() string { return e.message }

func (m *Module) githubWebhook(w http.ResponseWriter, r *http.Request) {
	if mediaType, _, err := mime.ParseMediaType(r.Header.Get("Content-Type")); err != nil || mediaType != "application/json" {
		http.Error(w, "Content-Type must be application/json", http.StatusUnsupportedMediaType)
		return
	}
	id, err := projectID(r)
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
		} else {
			http.Error(w, "Invalid payload", http.StatusBadRequest)
		}
		return
	}
	result, err := m.service.HandleGitHubWebhook(r.Context(), id, r.Header.Get("X-GitHub-Event"), r.Header.Get("X-GitHub-Delivery"), r.Header.Get("X-Hub-Signature-256"), body)
	if err != nil {
		var webhookErr *webhookError
		if errors.As(err, &webhookErr) {
			http.Error(w, webhookErr.message, webhookErr.status)
			return
		}
		status := http.StatusServiceUnavailable
		if errors.Is(err, ErrDeploymentInProgress) {
			status = http.StatusConflict
		}
		http.Error(w, err.Error(), status)
		return
	}
	status := http.StatusOK
	if result.Status == "accepted" {
		status = http.StatusAccepted
	}
	projectJSON(w, status, result)
}

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

func (s *LocalService) HandleGitHubWebhook(ctx context.Context, id int64, event, deliveryID, signature string, body []byte) (WebhookResponse, error) {
	project, err := s.repo.Get(ctx, id)
	if err != nil {
		return WebhookResponse{}, err
	}
	if !validGitHubSignature(project.WebhookSecret, body, signature) {
		return WebhookResponse{}, &webhookError{http.StatusUnauthorized, "Unauthorized"}
	}
	if event == "ping" {
		return WebhookResponse{Status: "ok"}, nil
	}
	if event != "push" {
		return WebhookResponse{Status: "ignored", Reason: "event"}, nil
	}
	deliveryID = strings.TrimSpace(deliveryID)
	if deliveryID == "" {
		return WebhookResponse{}, &webhookError{http.StatusBadRequest, "Missing X-GitHub-Delivery"}
	}
	var payload githubPushPayload
	if err := json.Unmarshal(body, &payload); err != nil {
		return WebhookResponse{}, &webhookError{http.StatusBadRequest, "Invalid payload"}
	}
	if payload.Deleted {
		return WebhookResponse{Status: "ignored", Reason: "deleted"}, nil
	}
	branch := normalizedBranch(project.WebhookBranch)
	if payload.Ref != "refs/heads/"+branch {
		return WebhookResponse{Status: "ignored", Reason: "branch"}, nil
	}
	if !validGitCommitSHA(payload.After) {
		return WebhookResponse{}, &webhookError{http.StatusBadRequest, "Invalid commit SHA"}
	}
	if s.deployments == nil {
		return WebhookResponse{}, errDeploymentUnavailable
	}
	deployment, duplicate, err := s.deployments.StartGitHubDeployment(ctx, project, payload.After, payload.HeadCommit.Message, payload.HeadCommit.Author.Name, deliveryID)
	if err != nil {
		return WebhookResponse{}, err
	}
	status := "accepted"
	if duplicate {
		status = "duplicate"
	}
	return WebhookResponse{Status: status, DeploymentID: deployment.ID}, nil
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

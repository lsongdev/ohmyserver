package web

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

type apiService struct {
	Service
	created           Input
	project           Project
	deployment        Deployment
	createdDeployment bool
	rotated           bool
	webhookResponse   WebhookResponse
	webhookErr        error
	webhookArgs       []any
}

func (s *apiService) HandleGitHubWebhook(ctx context.Context, id int64, event, deliveryID, signature string, body []byte) (WebhookResponse, error) {
	s.webhookArgs = []any{id, event, deliveryID, signature, string(body)}
	return s.webhookResponse, s.webhookErr
}

func (s *apiService) CreateProjectAPI(_ context.Context, input Input) (Project, error) {
	s.created = input
	s.project = Project{ID: 2, Name: input.Name, WebhookSecret: "one-time-secret"}
	return s.project, nil
}

func TestGitHubWebhookRouteAndPayloadLimit(t *testing.T) {
	service := &apiService{webhookResponse: WebhookResponse{Status: "accepted", DeploymentID: 9}}
	handler := (&Module{service: service}).Handler()
	body := []byte(`{"ref":"refs/heads/main"}`)
	request := httptest.NewRequest(http.MethodPost, "/hooks/github/4", bytes.NewReader(body))
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("X-GitHub-Event", "push")
	request.Header.Set("X-GitHub-Delivery", "delivery-1")
	request.Header.Set("X-Hub-Signature-256", "signature")
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	if response.Code != http.StatusAccepted || !strings.Contains(response.Body.String(), `"deployment_id":9`) {
		t.Fatalf("webhook response: status=%d body=%s", response.Code, response.Body.String())
	}
	if len(service.webhookArgs) != 5 || service.webhookArgs[0] != int64(4) || service.webhookArgs[1] != "push" || service.webhookArgs[2] != "delivery-1" || service.webhookArgs[4] != string(body) {
		t.Fatalf("service received unexpected webhook data: %#v", service.webhookArgs)
	}

	large := bytes.Repeat([]byte("x"), githubWebhookMaxBody+1)
	request = httptest.NewRequest(http.MethodPost, "/hooks/github/4", bytes.NewReader(large))
	request.Header.Set("Content-Type", "application/json")
	response = httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	if response.Code != http.StatusRequestEntityTooLarge {
		t.Fatalf("oversized webhook status=%d", response.Code)
	}
}

func TestValidGitHubSignature(t *testing.T) {
	const secret = "It's a Secret to Everybody"
	const signature = "sha256=757107ea0eb2509fc211221cce984b8a37570b6d7586c22c46f4379c8b043e17"
	if !validGitHubSignature(secret, []byte("Hello, World!"), signature) || validGitHubSignature(secret, []byte("tampered"), signature) {
		t.Fatal("GitHub HMAC verification failed")
	}
	mac := hmac.New(sha256.New, []byte(secret))
	_, _ = mac.Write([]byte("Hello, World!"))
	if got := "sha256=" + hex.EncodeToString(mac.Sum(nil)); got != signature {
		t.Fatalf("signature vector mismatch: %s", got)
	}
}
func (s *apiService) Project(context.Context, int64) (Project, error) { return s.project, nil }
func (s *apiService) StartDeploymentAPI(context.Context, int64) (Deployment, error) {
	s.createdDeployment = true
	return Deployment{ID: 8, ProjectID: 2, Status: DeploymentPending}, nil
}
func (s *apiService) RotateWebhookSecret(context.Context, int64) (string, error) {
	s.rotated = true
	return "rotated-secret", nil
}

func TestProjectsAPIOnlyRevealsWebhookSecretOnce(t *testing.T) {
	service := &apiService{}
	handler := (&Module{service: service}).Handler()
	call := func(method, path, body string) *httptest.ResponseRecorder {
		t.Helper()
		w := httptest.NewRecorder()
		handler.ServeHTTP(w, httptest.NewRequest(method, path, strings.NewReader(body)))
		return w
	}
	if response := call(http.MethodPost, "/projects", `{"name":"  ","webhook_secret":"bad"}`); response.Code != http.StatusBadRequest {
		t.Fatalf("invalid project: status=%d body=%s", response.Code, response.Body.String())
	}
	created := call(http.MethodPost, "/projects", `{"name":"website"}`)
	if created.Code != http.StatusCreated || !strings.Contains(created.Body.String(), `"webhook_secret_once":"one-time-secret"`) || strings.Contains(created.Body.String(), `"webhook_secret":"`) {
		t.Fatalf("create response: status=%d body=%s", created.Code, created.Body.String())
	}
	detail := call(http.MethodGet, "/projects/2", "")
	if detail.Code != http.StatusOK || strings.Contains(detail.Body.String(), "one-time-secret") {
		t.Fatalf("project detail leaked secret: status=%d body=%s", detail.Code, detail.Body.String())
	}
	rotated := call(http.MethodPost, "/projects/2/webhook-secret", "")
	if rotated.Code != http.StatusCreated || !service.rotated || !strings.Contains(rotated.Body.String(), `"webhook_secret_once":"rotated-secret"`) {
		t.Fatalf("rotate secret: status=%d body=%s", rotated.Code, rotated.Body.String())
	}
	started := call(http.MethodPost, "/projects/2/deployments", "")
	if started.Code != http.StatusAccepted || !service.createdDeployment {
		t.Fatalf("start deployment: status=%d body=%s", started.Code, started.Body.String())
	}
}

func TestProjectsAPIRejectsInvalidIDs(t *testing.T) {
	handler := (&Module{service: &apiService{}}).Handler()
	for _, path := range []string{"/projects/nope", "/projects/0/deployments"} {
		w := httptest.NewRecorder()
		handler.ServeHTTP(w, httptest.NewRequest(http.MethodGet, path, nil))
		if w.Code != http.StatusBadRequest {
			t.Errorf("%s: status=%d body=%s", path, w.Code, w.Body.String())
		}
	}
}

package server

import (
	"bytes"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os/exec"
	"path/filepath"
	"strconv"
	"testing"
	"time"

	"github.com/lsongdev/ohmyserver/config"
)

func TestValidGitHubSignatureOfficialVector(t *testing.T) {
	const secret = "It's a Secret to Everybody"
	const signature = "sha256=757107ea0eb2509fc211221cce984b8a37570b6d7586c22c46f4379c8b043e17"
	if !validGitHubSignature(secret, []byte("Hello, World!"), signature) {
		t.Fatal("official GitHub signature vector was rejected")
	}
	if validGitHubSignature(secret, []byte("tampered"), signature) {
		t.Fatal("tampered payload was accepted")
	}
}

func TestGitHubWebhookAcceptsPushAndDeduplicatesDelivery(t *testing.T) {
	server := newProjectTestServer(t)
	repository := createProjectTestRepository(t)
	project, err := server.CreateProject("hook-test", "", repository)
	if err != nil {
		t.Fatal(err)
	}
	server.deployRuntime = &fakeDeploymentRuntime{}
	originalWorkspaceDir := config.WorkspaceDir
	config.WorkspaceDir = filepath.Join(t.TempDir(), "workspace")
	t.Cleanup(func() { config.WorkspaceDir = originalWorkspaceDir })

	commit := gitOutputForTest(t, repository, "rev-parse", "HEAD")
	payload := githubPushPayload{
		Ref:   "refs/heads/main",
		After: commit,
	}
	payload.HeadCommit.Message = "test commit"
	payload.HeadCommit.Author.Name = "Test User"
	body, err := json.Marshal(payload)
	if err != nil {
		t.Fatal(err)
	}

	response := serveGitHubWebhook(t, server, project, "push", "delivery-1", body, project.WebhookSecret)
	if response.Code != http.StatusAccepted {
		t.Fatalf("status = %d, body = %s", response.Code, response.Body.String())
	}
	var accepted struct {
		DeploymentID int64 `json:"deployment_id"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &accepted); err != nil {
		t.Fatal(err)
	}
	waitForDeploymentStatus(t, server, accepted.DeploymentID, DeploymentStatusCompleted)

	response = serveGitHubWebhook(t, server, project, "push", "delivery-1", body, project.WebhookSecret)
	if response.Code != http.StatusOK {
		t.Fatalf("duplicate status = %d, body = %s", response.Code, response.Body.String())
	}
	var duplicate struct {
		Status       string `json:"status"`
		DeploymentID int64  `json:"deployment_id"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &duplicate); err != nil {
		t.Fatal(err)
	}
	if duplicate.Status != "duplicate" || duplicate.DeploymentID != accepted.DeploymentID {
		t.Fatalf("duplicate response = %+v", duplicate)
	}
}

func TestGitHubWebhookRejectsInvalidSignatureAndIgnoresOtherBranch(t *testing.T) {
	server := newProjectTestServer(t)
	project, err := server.CreateProject("hook-validation", "", "unused")
	if err != nil {
		t.Fatal(err)
	}
	body := []byte(`{"ref":"refs/heads/feature","after":"0123456789012345678901234567890123456789"}`)

	response := serveGitHubWebhook(t, server, project, "push", "delivery-2", body, "wrong-secret")
	if response.Code != http.StatusUnauthorized {
		t.Fatalf("invalid signature status = %d", response.Code)
	}

	response = serveGitHubWebhook(t, server, project, "push", "delivery-2", body, project.WebhookSecret)
	if response.Code != http.StatusOK || !bytes.Contains(response.Body.Bytes(), []byte(`"reason":"branch"`)) {
		t.Fatalf("branch response = %d %s", response.Code, response.Body.String())
	}

	response = serveGitHubWebhook(t, server, project, "ping", "ping-1", []byte(`{}`), project.WebhookSecret)
	if response.Code != http.StatusOK || !bytes.Contains(response.Body.Bytes(), []byte(`"status":"ok"`)) {
		t.Fatalf("ping response = %d %s", response.Code, response.Body.String())
	}
}

func TestGitHubWebhookLimitsPayloadSize(t *testing.T) {
	server := newProjectTestServer(t)
	project, err := server.CreateProject("hook-limit", "", "unused")
	if err != nil {
		t.Fatal(err)
	}
	body := bytes.Repeat([]byte("x"), githubWebhookMaxBody+1)
	response := serveGitHubWebhook(t, server, project, "push", "large", body, project.WebhookSecret)
	if response.Code != http.StatusRequestEntityTooLarge {
		t.Fatalf("large payload status = %d", response.Code)
	}
}

func serveGitHubWebhook(t *testing.T, server *Server, project *Project, event, delivery string, body []byte, signingSecret string) *httptest.ResponseRecorder {
	t.Helper()
	request := httptest.NewRequest(http.MethodPost, "/hooks/github/"+strconv.FormatInt(project.ID, 10), bytes.NewReader(body))
	request.SetPathValue("id", strconv.FormatInt(project.ID, 10))
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("X-GitHub-Event", event)
	request.Header.Set("X-GitHub-Delivery", delivery)
	mac := hmac.New(sha256.New, []byte(signingSecret))
	_, _ = mac.Write(body)
	request.Header.Set("X-Hub-Signature-256", "sha256="+hex.EncodeToString(mac.Sum(nil)))
	response := httptest.NewRecorder()
	server.GitHubWebhookHandler(response, request)
	return response
}

func waitForDeploymentStatus(t *testing.T, server *Server, deploymentID int64, want DeploymentStatus) {
	t.Helper()
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		deployment, err := server.GetDeployment(deploymentID)
		if err == nil && deployment.Status == want {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	deployment, err := server.GetDeployment(deploymentID)
	t.Fatalf("deployment status = %v, error = %v; want %s", deployment, err, want)
}

func gitOutputForTest(t *testing.T, dir string, args ...string) string {
	t.Helper()
	cmdArgs := append([]string{"-C", dir}, args...)
	output, err := exec.Command("git", cmdArgs...).CombinedOutput()
	if err != nil {
		t.Fatal(err)
	}
	return string(bytes.TrimSpace(output))
}

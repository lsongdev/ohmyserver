package web

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"testing"
)

type webhookDeployments struct {
	DeploymentService
	called bool
}

func (d *webhookDeployments) StartGitHubDeployment(_ context.Context, project *Project, commit, message, author, delivery string) (*Deployment, bool, error) {
	d.called = true
	if project.ID != 7 || commit != "0123456789012345678901234567890123456789" || message != "push" || author != "alice" || delivery != "delivery-1" {
		return nil, false, errors.New("unexpected deployment data")
	}
	return &Deployment{ID: 11}, false, nil
}

type localStore struct {
	Store
	project Project
	created bool
	updated bool
}

func (s *localStore) List(context.Context) ([]Project, error) { return []Project{s.project}, nil }
func (s *localStore) Get(_ context.Context, id int64) (*Project, error) {
	if !s.created || s.project.ID != id {
		return nil, errors.New("not found")
	}
	copy := s.project
	return &copy, nil
}
func (s *localStore) Create(_ context.Context, project Project) (*Project, error) {
	project.ID = 7
	s.project, s.created = project, true
	return &project, nil
}
func (s *localStore) Update(_ context.Context, project Project) error {
	if !s.created || s.project.ID != project.ID {
		return errors.New("not found")
	}
	s.project, s.updated = project, true
	return nil
}
func (s *localStore) Delete(context.Context, int64) error { s.created = false; return nil }

func TestLocalServiceOwnsProjectValidationAndSecretLifecycle(t *testing.T) {
	store := &localStore{}
	service := NewLocalService(store, nil)
	created, err := service.CreateProjectAPI(context.Background(), Input{Name: "  website  ", Repo: " https://example.test/repo.git "})
	if err != nil {
		t.Fatal(err)
	}
	if created.Name != "website" || created.Repo != "https://example.test/repo.git" || created.WebhookBranch != "main" || len(created.WebhookSecret) != 64 {
		t.Fatalf("normalized project or generated secret incorrect: %+v", created)
	}
	if err := service.UpdateProjectAPI(context.Background(), created.ID, Input{Name: "website-v2", Repo: created.Repo}); err != nil {
		t.Fatal(err)
	}
	if !store.updated || store.project.WebhookSecret != created.WebhookSecret || store.project.WebhookBranch != "main" {
		t.Fatalf("update should preserve secret and default branch: %+v", store.project)
	}
	if _, err := service.CreateProjectAPI(context.Background(), Input{Name: "bad", WebhookBranch: "refs/heads/main"}); err == nil {
		t.Fatal("invalid webhook branch accepted")
	}
}

func TestLocalServiceValidatesAndStartsGitHubWebhook(t *testing.T) {
	store := &localStore{created: true, project: Project{ID: 7, WebhookSecret: "secret", WebhookBranch: "main"}}
	deployments := &webhookDeployments{}
	service := NewLocalService(store, deployments)
	body := []byte(`{"ref":"refs/heads/main","after":"0123456789012345678901234567890123456789","head_commit":{"message":"push","author":{"name":"alice"}}}`)
	mac := hmac.New(sha256.New, []byte("secret"))
	_, _ = mac.Write(body)
	signature := "sha256=" + hex.EncodeToString(mac.Sum(nil))
	result, err := service.HandleGitHubWebhook(context.Background(), 7, "push", "delivery-1", signature, body)
	if err != nil || result.Status != "accepted" || result.DeploymentID != 11 || !deployments.called {
		t.Fatalf("webhook result=%+v called=%v err=%v", result, deployments.called, err)
	}
	if _, err := service.HandleGitHubWebhook(context.Background(), 7, "push", "delivery-1", "sha256=bad", body); err == nil {
		t.Fatal("invalid signature accepted")
	}
	branchBody := []byte(`{"ref":"refs/heads/feature"}`)
	mac = hmac.New(sha256.New, []byte("secret"))
	_, _ = mac.Write(branchBody)
	branchSignature := "sha256=" + hex.EncodeToString(mac.Sum(nil))
	ignored, err := service.HandleGitHubWebhook(context.Background(), 7, "push", "delivery-2", branchSignature, branchBody)
	if err != nil || ignored.Status != "ignored" || ignored.Reason != "branch" {
		t.Fatalf("wrong branch result=%+v err=%v", ignored, err)
	}
}

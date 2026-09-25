package web

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"
)

var errDeploymentUnavailable = errors.New("project deployment runtime is unavailable")

type LocalService struct {
	repo        Store
	deployments DeploymentService
}

func NewLocalService(repo Store, deployments DeploymentService) *LocalService {
	return &LocalService{repo: repo, deployments: deployments}
}

func (s *LocalService) Projects(ctx context.Context) ([]Project, error) {
	return s.repo.List(ctx)
}
func (s *LocalService) Project(ctx context.Context, id int64) (Project, error) {
	item, err := s.repo.Get(ctx, id)
	if err != nil {
		return Project{}, err
	}
	return *item, nil
}
func (s *LocalService) Deployments(ctx context.Context, id int64) ([]Deployment, error) {
	return s.repo.DeploymentsByProject(ctx, id)
}
func (s *LocalService) Deployment(ctx context.Context, id int64) (Deployment, error) {
	item, err := s.repo.GetDeployment(ctx, id)
	if err != nil {
		return Deployment{}, err
	}
	return *item, nil
}
func (s *LocalService) DeploymentLogs(ctx context.Context, id int64) ([]DeploymentLog, error) {
	return s.repo.DeploymentLogs(ctx, id)
}
func (s *LocalService) StartDeploymentAPI(ctx context.Context, id int64) (Deployment, error) {
	if err := ctx.Err(); err != nil {
		return Deployment{}, err
	}
	project, err := s.repo.Get(ctx, id)
	if err != nil {
		return Deployment{}, err
	}
	if strings.TrimSpace(project.Repo) == "" {
		return Deployment{}, errors.New("repository URL is required")
	}
	if s.deployments == nil {
		return Deployment{}, errDeploymentUnavailable
	}
	return s.deployments.StartDeploymentAPI(ctx, id)
}
func (s *LocalService) CancelDeploymentAPI(ctx context.Context, projectID, deploymentID int64) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if s.deployments == nil {
		return errDeploymentUnavailable
	}
	return s.deployments.CancelDeploymentAPI(ctx, projectID, deploymentID)
}
func (s *LocalService) ProjectRuntime(ctx context.Context, id int64) (RuntimeStatus, error) {
	if _, err := s.repo.Get(ctx, id); err != nil {
		return RuntimeStatus{}, err
	}
	if s.deployments == nil {
		return RuntimeStatus{}, errDeploymentUnavailable
	}
	return s.deployments.ProjectRuntime(ctx, id)
}
func (s *LocalService) RotateWebhookSecret(ctx context.Context, id int64) (string, error) {
	project, err := s.repo.Get(ctx, id)
	if err != nil {
		return "", err
	}
	secret, err := generateWebhookSecret()
	if err != nil {
		return "", err
	}
	project.WebhookSecret = secret
	if err := s.repo.Update(ctx, *project); err != nil {
		return "", err
	}
	return secret, nil
}
func (s *LocalService) CreateProjectAPI(ctx context.Context, in Input) (Project, error) {
	if err := ctx.Err(); err != nil {
		return Project{}, err
	}
	branch := normalizedBranch(in.WebhookBranch)
	if !validBranch(branch) {
		return Project{}, errors.New("invalid GitHub webhook branch")
	}
	secret := strings.TrimSpace(in.WebhookSecret)
	if secret == "" {
		var err error
		secret, err = generateWebhookSecret()
		if err != nil {
			return Project{}, err
		}
	}
	project := Project{Name: strings.TrimSpace(in.Name), Description: in.Description, Repo: strings.TrimSpace(in.Repo), WebhookBranch: branch, WebhookSecret: secret}
	created, err := s.repo.Create(ctx, project)
	if err != nil {
		return Project{}, err
	}
	return *created, nil
}
func (s *LocalService) UpdateProjectAPI(ctx context.Context, id int64, in Input) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	old, err := s.repo.Get(ctx, id)
	if err != nil {
		return err
	}
	branch := normalizedBranch(in.WebhookBranch)
	if !validBranch(branch) {
		return errors.New("invalid GitHub webhook branch")
	}
	secret := strings.TrimSpace(in.WebhookSecret)
	if secret == "" {
		secret = old.WebhookSecret
	}
	return s.repo.Update(ctx, Project{ID: id, Name: strings.TrimSpace(in.Name), Description: in.Description, Repo: strings.TrimSpace(in.Repo), WebhookBranch: branch, WebhookSecret: secret})
}
func (s *LocalService) DeleteProjectAPI(ctx context.Context, id int64) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	return s.repo.Delete(ctx, id)
}

func normalizedBranch(branch string) string {
	if branch = strings.TrimSpace(branch); branch == "" {
		return "main"
	}
	return branch
}
func validBranch(value string) bool {
	return value != "" && !strings.HasPrefix(value, "-") && !strings.HasPrefix(value, "refs/") && !strings.HasPrefix(value, "/") && !strings.HasSuffix(value, "/") && !strings.HasSuffix(value, ".") && !strings.HasSuffix(value, ".lock") && !strings.Contains(value, "..") && !strings.Contains(value, "//") && !strings.Contains(value, "@{") && !strings.ContainsAny(value, " ~^:?*[\\")
}
func generateWebhookSecret() (string, error) {
	secret := make([]byte, 32)
	if _, err := rand.Read(secret); err != nil {
		return "", fmt.Errorf("generate webhook secret: %w", err)
	}
	return hex.EncodeToString(secret), nil
}

var _ Service = (*LocalService)(nil)

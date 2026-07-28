package server

import (
	"context"
	"database/sql"
	"errors"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/docker/docker/api/types"
	"github.com/lsongdev/ohmyserver/config"
)

type fakeDeploymentRuntime struct {
	buildStarted   chan struct{}
	buildStartOnce sync.Once
	waitForCancel  bool
	buildImage     string
	createdImage   string
	createdName    string
	startedID      string
	inspected      *types.ContainerJSON
}

func (f *fakeDeploymentRuntime) BuildImageFromDirContext(ctx context.Context, imageName, buildDir string, onLog func(string)) error {
	f.buildImage = imageName
	if _, err := os.Stat(filepath.Join(buildDir, "Dockerfile")); err != nil {
		return err
	}
	if f.buildStarted != nil {
		f.buildStartOnce.Do(func() { close(f.buildStarted) })
	}
	if f.waitForCancel {
		<-ctx.Done()
		return ctx.Err()
	}
	onLog("fake build complete")
	return nil
}

func (f *fakeDeploymentRuntime) GetContainer(string) (types.ContainerJSON, error) {
	if f.inspected != nil {
		return *f.inspected, nil
	}
	return types.ContainerJSON{}, errors.New("not found")
}

func (f *fakeDeploymentRuntime) RemoveContainer(string) error {
	return nil
}

func (f *fakeDeploymentRuntime) CreateContainer(imageName, containerName string, _ []string) (string, error) {
	f.createdImage = imageName
	f.createdName = containerName
	return "container-id", nil
}

func (f *fakeDeploymentRuntime) StartContainer(id string) error {
	f.startedID = id
	return nil
}

func TestDeploymentExecutorBuildsAndDeploysCommit(t *testing.T) {
	server := newProjectTestServer(t)
	repository := createProjectTestRepository(t)
	project, err := server.CreateProject("unsafe/../name", "", repository)
	if err != nil {
		t.Fatal(err)
	}
	deployment, err := server.CreateDeployment(project.ID, "", "", "", "", "", "", "", "Test")
	if err != nil {
		t.Fatal(err)
	}

	originalWorkspaceDir := config.WorkspaceDir
	config.WorkspaceDir = t.TempDir()
	t.Cleanup(func() { config.WorkspaceDir = originalWorkspaceDir })

	runtime := &fakeDeploymentRuntime{}
	executor := NewDeploymentExecutor(server, deployment)
	executor.runtime = runtime
	if err := executor.Execute(); err != nil {
		t.Fatalf("execute deployment: %v", err)
	}

	stored, err := server.GetDeployment(deployment.ID)
	if err != nil {
		t.Fatal(err)
	}
	if stored.Status != DeploymentStatusCompleted {
		t.Fatalf("status = %s, want completed", stored.Status)
	}
	if stored.Commit == "" || stored.CommitMsg != "test commit" || stored.Author != "Test User" {
		t.Fatalf("source metadata = %q, %q, %q", stored.Commit, stored.CommitMsg, stored.Author)
	}
	if stored.DockerImage != "ohmyserver/project-1:deployment-1" {
		t.Fatalf("image = %q", stored.DockerImage)
	}
	if stored.ContainerName != "ohmyserver-project-1" || stored.ContainerID != "container-id" {
		t.Fatalf("container = %q (%q)", stored.ContainerName, stored.ContainerID)
	}
	if runtime.createdImage != stored.DockerImage || runtime.startedID != "container-id" {
		t.Fatalf("runtime did not deploy recorded artifact")
	}
	if _, err := os.Stat(executor.workDir); !os.IsNotExist(err) {
		t.Fatalf("work directory was not removed: %v", err)
	}
}

func TestDeploymentExecutorCancellationIsNotFailure(t *testing.T) {
	server := newProjectTestServer(t)
	repository := createProjectTestRepository(t)
	project, err := server.CreateProject("cancel-test", "", repository)
	if err != nil {
		t.Fatal(err)
	}
	deployment, err := server.CreateDeployment(project.ID, "", "", "", "", "", "", "", "Test")
	if err != nil {
		t.Fatal(err)
	}

	originalWorkspaceDir := config.WorkspaceDir
	config.WorkspaceDir = t.TempDir()
	t.Cleanup(func() { config.WorkspaceDir = originalWorkspaceDir })

	runtime := &fakeDeploymentRuntime{
		buildStarted:  make(chan struct{}),
		waitForCancel: true,
	}
	buildStarted := runtime.buildStarted
	executor := NewDeploymentExecutor(server, deployment)
	executor.runtime = runtime
	done := make(chan error, 1)
	go func() {
		done <- executor.Execute()
	}()
	<-buildStarted
	executor.Cancel()
	if err := <-done; !errors.Is(err, context.Canceled) {
		t.Fatalf("execute error = %v, want context.Canceled", err)
	}

	stored, err := server.GetDeployment(deployment.ID)
	if err != nil {
		t.Fatal(err)
	}
	if stored.Status != DeploymentStatusCancelled {
		t.Fatalf("status = %s, want cancelled", stored.Status)
	}
}

func TestRecoverInterruptedDeployments(t *testing.T) {
	server := newProjectTestServer(t)
	project, err := server.CreateProject("recover-test", "", "repo")
	if err != nil {
		t.Fatal(err)
	}
	deployment, err := server.CreateDeployment(project.ID, "", "", "", "", "", "", "", "Test")
	if err != nil {
		t.Fatal(err)
	}
	if err := server.UpdateDeploymentStatus(deployment.ID, DeploymentStatusRunning); err != nil {
		t.Fatal(err)
	}

	if err := server.RecoverInterruptedDeployments(); err != nil {
		t.Fatal(err)
	}
	stored, err := server.GetDeployment(deployment.ID)
	if err != nil {
		t.Fatal(err)
	}
	if stored.Status != DeploymentStatusFailed || stored.CompletedAt == nil {
		t.Fatalf("recovered deployment = %s, completed %v", stored.Status, stored.CompletedAt)
	}
	logs, err := server.GetLogsByDeployment(deployment.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(logs) != 1 {
		t.Fatalf("recovery logs = %d, want 1", len(logs))
	}
}

func TestProjectRejectsConcurrentDeployment(t *testing.T) {
	server := newProjectTestServer(t)
	project, err := server.CreateProject("single-flight", "", "repo")
	if err != nil {
		t.Fatal(err)
	}
	active, err := server.CreateDeployment(project.ID, "", "", "", "", "", "", "", "Test")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := server.StartProjectDeployment(project, "Manual"); err == nil {
		t.Fatal("concurrent deployment was accepted")
	}
	stored, err := server.GetActiveDeploymentByProject(project.ID)
	if err != nil {
		t.Fatal(err)
	}
	if stored == nil || stored.ID != active.ID {
		t.Fatalf("active deployment = %+v", stored)
	}
}

func TestFailedBuildKeepsPreviousRuntimeVisible(t *testing.T) {
	server := newProjectTestServer(t)
	project, err := server.CreateProject("runtime-test", "", "repo")
	if err != nil {
		t.Fatal(err)
	}
	successful, err := server.CreateDeployment(project.ID, "", "", "", "", "", "", "", "Test")
	if err != nil {
		t.Fatal(err)
	}
	if err := server.UpdateDeploymentArtifact(successful.ID, "image", "container"); err != nil {
		t.Fatal(err)
	}
	if err := server.UpdateDeploymentContainerID(successful.ID, "container-id"); err != nil {
		t.Fatal(err)
	}
	if err := server.UpdateDeploymentStatus(successful.ID, DeploymentStatusCompleted); err != nil {
		t.Fatal(err)
	}
	failed, err := server.CreateDeployment(project.ID, "", "", "", "", "", "", "", "Test")
	if err != nil {
		t.Fatal(err)
	}
	if err := server.UpdateDeploymentStatus(failed.ID, DeploymentStatusFailed); err != nil {
		t.Fatal(err)
	}

	server.deployRuntime = &fakeDeploymentRuntime{
		inspected: &types.ContainerJSON{
			ContainerJSONBase: &types.ContainerJSONBase{
				ID:    "container-id",
				State: &types.ContainerState{Running: true},
			},
		},
	}
	status, err := server.GetProjectRuntimeStatus(project.ID)
	if err != nil {
		t.Fatal(err)
	}
	if status.Status != "running" || status.ContainerID != "container-id" {
		t.Fatalf("runtime status = %+v", status)
	}
}

func TestProjectTemplates(t *testing.T) {
	now := time.Now()
	project := &Project{
		ID:            1,
		Name:          "example",
		Repo:          "https://github.com/example/project.git",
		WebhookSecret: "secret",
		WebhookBranch: "main",
		CreatedAt:     now,
		UpdatedAt:     now,
	}
	tests := []struct {
		template string
		data     H
	}{
		{"projects/index", H{"projects": []Project{*project}}},
		{"projects/new", H{}},
		{"projects/settings", H{"project": project, "webhookURL": "https://example.test/hooks/github/1"}},
		{"projects/detail", H{
			"project": project, "runtimeStatus": &ProjectRuntimeStatus{Status: "not_deployed"},
		}},
		{"projects/deployments", H{"project": project, "deployments": []H{}}},
		{"projects/deployment_detail", H{
			"project": project,
			"deployment": H{
				"ID": 1, "Status": DeploymentStatusRunning, "TriggeredBy": "Test",
				"LastLogID": int64(0), "Logs": []string{},
			},
		}},
	}
	for _, test := range tests {
		t.Run(test.template, func(t *testing.T) {
			response := httptest.NewRecorder()
			(&Server{}).Render(response, test.template, test.data)
			if response.Code != 200 {
				t.Fatalf("status = %d, body = %s", response.Code, response.Body.String())
			}
		})
	}
}

func TestCICDSchemaMigratesExistingDatabase(t *testing.T) {
	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	db.SetMaxOpenConns(1)
	t.Cleanup(func() { db.Close() })
	if _, err := db.Exec(`
		CREATE TABLE projects (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			name TEXT NOT NULL UNIQUE,
			description TEXT,
			repo TEXT DEFAULT '',
			created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
			updated_at DATETIME DEFAULT CURRENT_TIMESTAMP
		);
		CREATE TABLE deployments (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			project_id INTEGER NOT NULL,
			status TEXT NOT NULL DEFAULT 'pending',
			docker_image TEXT DEFAULT '',
			dockerfile TEXT,
			container_name TEXT DEFAULT '',
			container_id TEXT DEFAULT '',
			command TEXT DEFAULT '',
			"commit" TEXT DEFAULT '',
			commit_msg TEXT DEFAULT '',
			author TEXT DEFAULT '',
			triggered_by TEXT DEFAULT 'Manual',
			started_at DATETIME,
			completed_at DATETIME,
			created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
			updated_at DATETIME DEFAULT CURRENT_TIMESTAMP
		);
		INSERT INTO projects (name, description, repo) VALUES ('legacy', '', 'repo');
	`); err != nil {
		t.Fatal(err)
	}

	server := &Server{db: db}
	if err := server.initSchema(); err != nil {
		t.Fatal(err)
	}
	project, err := server.GetProjectByID(1)
	if err != nil {
		t.Fatal(err)
	}
	if project.WebhookBranch != "main" || len(project.WebhookSecret) != 64 {
		t.Fatalf("migrated webhook settings = %q, %q", project.WebhookBranch, project.WebhookSecret)
	}
	if _, err := server.CreateGitHubDeployment(
		project.ID,
		"0123456789012345678901234567890123456789",
		"message",
		"author",
		"delivery",
	); err != nil {
		t.Fatalf("create migrated GitHub deployment: %v", err)
	}
}

func newProjectTestServer(t *testing.T) *Server {
	t.Helper()
	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	db.SetMaxOpenConns(1)
	t.Cleanup(func() { db.Close() })
	server := &Server{
		db:          db,
		deployments: make(map[int64]*DeploymentExecutor),
	}
	if err := server.initSchema(); err != nil {
		t.Fatal(err)
	}
	return server
}

func createProjectTestRepository(t *testing.T) string {
	t.Helper()
	repository := t.TempDir()
	gitForTest(t, repository, "init", "-b", "main")
	gitForTest(t, repository, "config", "user.email", "test@example.com")
	gitForTest(t, repository, "config", "user.name", "Test User")
	if err := os.WriteFile(filepath.Join(repository, "Dockerfile"), []byte("FROM scratch\n"), 0644); err != nil {
		t.Fatal(err)
	}
	gitForTest(t, repository, "add", "Dockerfile")
	gitForTest(t, repository, "commit", "-m", "test commit")
	return repository
}

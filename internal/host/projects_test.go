package host

import (
	"context"
	"database/sql"
	"errors"
	"os"
	"path/filepath"
	"sync"
	"testing"

	"github.com/docker/docker/api/types"
	"github.com/lsongdev/ohmyserver/config"
	projectsmodule "github.com/lsongdev/ohmyserver/modules/projects"
	projectsweb "github.com/lsongdev/ohmyserver/modules/projects/web"
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

func (f *fakeDeploymentRuntime) InspectContainer(_ context.Context, name string) (projectsweb.ContainerState, error) {
	container, err := f.GetContainer(name)
	if err != nil {
		return projectsweb.ContainerState{}, err
	}
	return projectsweb.ContainerState{ID: container.ID, Running: container.State.Running, Dead: container.State.Dead, OOMKilled: container.State.OOMKilled, ExitCode: container.State.ExitCode}, nil
}

func (f *fakeDeploymentRuntime) RemoveContainer(context.Context, string) error {
	return nil
}

func (f *fakeDeploymentRuntime) CreateContainer(_ context.Context, imageName, containerName string, _ []string) (string, error) {
	f.createdImage = imageName
	f.createdName = containerName
	return "container-id", nil
}

func (f *fakeDeploymentRuntime) StartContainer(_ context.Context, id string) error {
	f.startedID = id
	return nil
}

func TestDeploymentExecutorBuildsAndDeploysCommit(t *testing.T) {
	server := newProjectTestServer(t)
	repository := createProjectTestRepository(t)
	project, err := createTestProject(t, server, "unsafe/../name", repository)
	if err != nil {
		t.Fatal(err)
	}
	deployment, err := createTestDeployment(t, server, project.ID)
	if err != nil {
		t.Fatal(err)
	}

	originalWorkspaceDir := config.WorkspaceDir
	config.WorkspaceDir = t.TempDir()
	t.Cleanup(func() { config.WorkspaceDir = originalWorkspaceDir })

	runtime := &fakeDeploymentRuntime{}
	executor := projectsweb.NewDeploymentExecutor(context.Background(), server.projectRepository(), runtime, config.WorkspaceDir, deployment)
	if err := executor.Execute(); err != nil {
		t.Fatalf("execute deployment: %v", err)
	}

	stored, err := server.projectRepository().GetDeployment(context.Background(), deployment.ID)
	if err != nil {
		t.Fatal(err)
	}
	if stored.Status != projectsmodule.DeploymentCompleted {
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
	workDir := filepath.Join(config.WorkspaceDir, "projects", "1", "1")
	if _, err := os.Stat(workDir); !os.IsNotExist(err) {
		t.Fatalf("work directory was not removed: %v", err)
	}
}

func TestDeploymentExecutorCancellationIsNotFailure(t *testing.T) {
	server := newProjectTestServer(t)
	repository := createProjectTestRepository(t)
	project, err := createTestProject(t, server, "cancel-test", repository)
	if err != nil {
		t.Fatal(err)
	}
	deployment, err := createTestDeployment(t, server, project.ID)
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
	executor := projectsweb.NewDeploymentExecutor(context.Background(), server.projectRepository(), runtime, config.WorkspaceDir, deployment)
	done := make(chan error, 1)
	go func() {
		done <- executor.Execute()
	}()
	<-buildStarted
	executor.Cancel()
	if err := <-done; !errors.Is(err, context.Canceled) {
		t.Fatalf("execute error = %v, want context.Canceled", err)
	}

	stored, err := server.projectRepository().GetDeployment(context.Background(), deployment.ID)
	if err != nil {
		t.Fatal(err)
	}
	if stored.Status != projectsmodule.DeploymentCancelled {
		t.Fatalf("status = %s, want cancelled", stored.Status)
	}
}

func TestRecoverInterruptedDeployments(t *testing.T) {
	server := newProjectTestServer(t)
	project, err := createTestProject(t, server, "recover-test", "repo")
	if err != nil {
		t.Fatal(err)
	}
	deployment, err := createTestDeployment(t, server, project.ID)
	if err != nil {
		t.Fatal(err)
	}
	if err := server.projectRepository().UpdateDeploymentStatus(context.Background(), deployment.ID, projectsmodule.DeploymentRunning); err != nil {
		t.Fatal(err)
	}

	if err := server.projectRepository().RecoverInterruptedDeployments(context.Background()); err != nil {
		t.Fatal(err)
	}
	stored, err := server.projectRepository().GetDeployment(context.Background(), deployment.ID)
	if err != nil {
		t.Fatal(err)
	}
	if stored.Status != projectsmodule.DeploymentFailed || stored.CompletedAt == nil {
		t.Fatalf("recovered deployment = %s, completed %v", stored.Status, stored.CompletedAt)
	}
	logs, err := server.projectRepository().DeploymentLogs(context.Background(), deployment.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(logs) != 1 {
		t.Fatalf("recovery logs = %d, want 1", len(logs))
	}
}

func TestProjectRejectsConcurrentDeployment(t *testing.T) {
	server := newProjectTestServer(t)
	project, err := createTestProject(t, server, "single-flight", "repo")
	if err != nil {
		t.Fatal(err)
	}
	active, err := createTestDeployment(t, server, project.ID)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := server.ProjectDeploymentManager().StartProjectDeployment(context.Background(), project, "Manual"); err == nil {
		t.Fatal("concurrent deployment was accepted")
	}
	stored, err := server.projectRepository().ActiveDeployment(context.Background(), project.ID)
	if err != nil {
		t.Fatal(err)
	}
	if stored == nil || stored.ID != active.ID {
		t.Fatalf("active deployment = %+v", stored)
	}
}

func TestFailedBuildKeepsPreviousRuntimeVisible(t *testing.T) {
	server := newProjectTestServer(t)
	project, err := createTestProject(t, server, "runtime-test", "repo")
	if err != nil {
		t.Fatal(err)
	}
	successful, err := createTestDeployment(t, server, project.ID)
	if err != nil {
		t.Fatal(err)
	}
	if err := server.projectRepository().UpdateDeploymentArtifact(context.Background(), successful.ID, "image", "container"); err != nil {
		t.Fatal(err)
	}
	if err := server.projectRepository().UpdateDeploymentContainer(context.Background(), successful.ID, "container-id"); err != nil {
		t.Fatal(err)
	}
	if err := server.projectRepository().UpdateDeploymentStatus(context.Background(), successful.ID, projectsmodule.DeploymentCompleted); err != nil {
		t.Fatal(err)
	}
	failed, err := createTestDeployment(t, server, project.ID)
	if err != nil {
		t.Fatal(err)
	}
	if err := server.projectRepository().UpdateDeploymentStatus(context.Background(), failed.ID, projectsmodule.DeploymentFailed); err != nil {
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
	status, err := server.ProjectDeploymentManager().ProjectRuntime(context.Background(), project.ID)
	if err != nil {
		t.Fatal(err)
	}
	if status.Status != "running" || status.ContainerID != "container-id" {
		t.Fatalf("runtime status = %+v", status)
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
	project, err := server.projectRepository().Get(context.Background(), 1)
	if err != nil {
		t.Fatal(err)
	}
	if project.WebhookBranch != "main" || len(project.WebhookSecret) != 64 {
		t.Fatalf("migrated webhook settings = %q, %q", project.WebhookBranch, project.WebhookSecret)
	}
	if _, err := server.projectRepository().CreateGitHubDeployment(context.Background(), projectsmodule.Deployment{
		ProjectID: project.ID, Commit: "0123456789012345678901234567890123456789", CommitMsg: "message", Author: "author",
	}, "delivery"); err != nil {
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
		db: db,
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

func createTestProject(t *testing.T, server *Server, name, repo string) (*projectsmodule.Project, error) {
	t.Helper()
	service := projectsweb.NewLocalService(server.projectRepository(), server.ProjectDeploymentManager())
	project, err := service.CreateProjectAPI(context.Background(), projectsweb.Input{Name: name, Repo: repo})
	if err != nil {
		return nil, err
	}
	return &project, nil
}

func createTestDeployment(t *testing.T, server *Server, projectID int64) (*projectsmodule.Deployment, error) {
	t.Helper()
	return server.projectRepository().CreateDeployment(context.Background(), projectsmodule.Deployment{ProjectID: projectID, TriggeredBy: "Test"})
}

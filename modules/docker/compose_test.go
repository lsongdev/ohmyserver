package docker

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"
)

func TestParseComposeContainersAcceptsV1JSONLinesAndV2JSONArray(t *testing.T) {
	for _, output := range []string{
		"{\"Name\":\"web-1\"}\n{\"Name\":\"db-1\"}\n",
		"[{\"Name\":\"web-1\"},{\"Name\":\"db-1\"}]\n",
	} {
		items := parseComposeContainers([]byte(output))
		if len(items) != 2 || items[0]["Name"] != "web-1" || items[1]["Name"] != "db-1" {
			t.Fatalf("parsed containers = %#v", items)
		}
	}
	if got := parseComposeContainers(nil); got == nil || len(got) != 0 {
		t.Fatalf("empty output = %#v, want non-nil empty list", got)
	}
}

func TestDockerComposeCommandFallsBackToDockerComposePlugin(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("uses a temporary POSIX command shim")
	}
	bin := t.TempDir()
	if err := os.WriteFile(filepath.Join(bin, "docker"), []byte("#!/bin/sh\nprintf '%s\\n' \"$*\"\n"), 0755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", bin)
	output, err := dockerComposeCommand(context.Background(), "ls", "--format", "json").CombinedOutput()
	if err != nil {
		t.Fatalf("run Docker Compose plugin command: %v: %s", err, output)
	}
	if got, want := strings.TrimSpace(string(output)), "compose ls --format json"; got != want {
		t.Fatalf("plugin arguments = %q, want %q", got, want)
	}
}

func TestComposeServiceUsesContextAndSupportsLifecycle(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("uses a temporary POSIX command shim")
	}
	root := t.TempDir()
	bin := filepath.Join(root, "bin")
	if err := os.Mkdir(bin, 0755); err != nil {
		t.Fatal(err)
	}
	logFile := filepath.Join(root, "calls.log")
	shim := "#!/bin/sh\nprintf '%s\\n' \"$*\" >> \"$DOCKER_COMPOSE_TEST_LOG\"\nif [ \"$DOCKER_COMPOSE_TEST_DELAY\" = 1 ]; then exec /bin/sleep 5; fi\ncase \"$*\" in\n  *\"ls --format json --all\"*) cat \"$DOCKER_COMPOSE_TEST_LIST\" ;;\n  *\"ps --format json\"*) cat \"$DOCKER_COMPOSE_TEST_PS\" ;;\nesac\n"
	if err := os.WriteFile(filepath.Join(bin, "docker-compose"), []byte(shim), 0755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", bin+string(os.PathListSeparator)+os.Getenv("PATH"))
	t.Setenv("DOCKER_COMPOSE_TEST_LOG", logFile)

	composeRoot := filepath.Join(root, "compose")
	service := NewComposeService(composeRoot)
	name := "compose-qa"
	composeFile := filepath.Join(composeRoot, name, "compose.yaml")
	projectsFile := filepath.Join(root, "projects.json")
	containersFile := filepath.Join(root, "containers.json")
	projectJSON, err := json.Marshal([]map[string]string{{"Name": name, "Status": "running(1)", "ConfigFiles": composeFile}})
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(projectsFile, projectJSON, 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(containersFile, []byte("[{\"Name\":\"compose-qa-web-1\",\"State\":\"running\"}]\n"), 0600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("DOCKER_COMPOSE_TEST_LIST", projectsFile)
	t.Setenv("DOCKER_COMPOSE_TEST_PS", containersFile)

	ctx := context.Background()
	created, err := service.CreateDockerComposeProject(ctx, ComposeProjectRequest{Name: name, Source: "file", Compose: "services: {}\n"})
	if err != nil || created.Name != name || created.ConfigFiles != composeFile {
		t.Fatalf("create project = %+v, err = %v", created, err)
	}
	if contents, err := os.ReadFile(composeFile); err != nil || string(contents) != "services: {}\n" {
		t.Fatalf("created Compose file = %q, err=%v", contents, err)
	}
	projects, err := service.ListDockerComposeProjects(ctx)
	if err != nil || len(projects) != 1 || projects[0].Name != name {
		t.Fatalf("list projects = %+v, err=%v", projects, err)
	}
	detail, err := service.GetDockerComposeProject(ctx, name)
	if err != nil || detail.Project.Name != name || len(detail.Containers) != 1 || detail.Containers[0]["Name"] != "compose-qa-web-1" {
		t.Fatalf("project detail = %+v, err=%v", detail, err)
	}
	if err := service.DockerComposeAction(ctx, name, "down"); err != nil {
		t.Fatalf("Compose down action: %v", err)
	}
	canceled, cancel := context.WithCancel(ctx)
	cancel()
	if _, err := service.ListDockerComposeProjects(canceled); err == nil {
		t.Fatal("list should honor an already-canceled request context")
	}
	t.Setenv("DOCKER_COMPOSE_TEST_DELAY", "1")
	commandCtx, commandCancel := context.WithTimeout(ctx, 50*time.Millisecond)
	defer commandCancel()
	started := time.Now()
	err = runCompose(commandCtx, name, composeFile, "down")
	if err == nil || !strings.Contains(err.Error(), "deadline exceeded") {
		t.Fatalf("Compose CLI cancellation error = %v", err)
	}
	if elapsed := time.Since(started); elapsed > time.Second {
		t.Fatalf("Compose CLI ignored request cancellation for %s", elapsed)
	}
	calls, err := os.ReadFile(logFile)
	if err != nil {
		t.Fatal(err)
	}
	for _, expected := range []string{"-f " + composeFile + " -p " + name + " up -d", "ls --format json --all", "-p " + name + " ps --format json", "-f " + composeFile + " -p " + name + " down"} {
		if !strings.Contains(string(calls), expected) {
			t.Errorf("Compose command log does not include %q:\n%s", expected, calls)
		}
	}
}

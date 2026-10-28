package system

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

// DockerSource is a repository to install Docker Engine from.
type DockerSource struct {
	ID    string `json:"id"`
	Title string `json:"title"`
	URL   string `json:"url"`
}

// DockerSources are Docker's own repository and a mirror for hosts in
// mainland China, where download.docker.com is often slow or unreachable.
var DockerSources = []DockerSource{
	{ID: "docker", Title: "Docker (download.docker.com)", URL: "https://download.docker.com"},
	{ID: "aliyun", Title: "Alibaba Cloud mirror (mirrors.aliyun.com)", URL: "https://mirrors.aliyun.com/docker-ce"},
}

// dockerPackages are Docker Engine, its CLI and runtime, and the Buildx
// and Compose plugins.
var dockerPackages = []string{"docker-ce", "docker-ce-cli", "containerd.io", "docker-buildx-plugin", "docker-compose-plugin"}

// Docker installs Docker Engine from Docker's repositories, following
// https://docs.docker.com/engine/install/.
type Docker struct {
	Manager Manager
	OS      OS
	Run     Runner
	Root    string       // file system root, "/" on a real host
	HTTP    *http.Client // fetches apt's signing key
}

// DockerStatus is whether Docker is installed and running here.
type DockerStatus struct {
	Installed bool   `json:"installed"`
	Running   bool   `json:"running"`
	Version   string `json:"version,omitempty"`
	// Unsupported says why the recipe cannot install Docker here, if so.
	Unsupported string         `json:"unsupported,omitempty"`
	Sources     []DockerSource `json:"sources"`
}

// Status checks for the docker CLI and asks systemd whether the service
// runs.
func (d *Docker) Status(ctx context.Context) DockerStatus {
	s := DockerStatus{Sources: DockerSources}
	if _, err := d.repository(DockerSources[0].URL); err != nil {
		s.Unsupported = err.Error()
	}
	if _, err := exec.LookPath("docker"); err == nil {
		s.Installed = true
		// "Docker version 29.0.1, build eedd969", even with the daemon down.
		if out, err := d.Run.Output(ctx, nil, "docker", "--version"); err == nil {
			version, _, _ := strings.Cut(strings.TrimPrefix(strings.TrimSpace(string(out)), "Docker version "), ",")
			s.Version = version
		}
	}
	if d.Manager != nil && d.Manager.Name() == "brew" {
		// No systemd: ask the daemon (Docker Desktop, OrbStack, colima).
		if s.Installed {
			_, err := d.Run.Output(ctx, nil, "docker", "info", "--format", "{{.ServerVersion}}")
			s.Running = err == nil
		}
	} else if out, err := d.Run.Output(ctx, nil, "systemctl", "is-active", "docker"); err == nil || len(out) > 0 {
		s.Running = strings.TrimSpace(string(out)) == "active"
	}
	return s
}

// repository describes where the packages for this distribution are.
type repository struct {
	file    string // repository definition, relative to the root
	content string
	keyURL  string // apt only: the signing key, saved to keyFile
	keyFile string
}

func (d *Docker) repository(base string) (repository, error) {
	switch {
	case d.Manager == nil:
		return repository{}, ErrUnsupported
	case d.Manager.Name() == "dnf":
		distro, release := "centos", "$releasever"
		if d.OS.ID == "fedora" {
			distro = "fedora"
		} else if major := d.OS.ELMajor(); major > 0 {
			// Derivatives number their releases differently (Alibaba Cloud
			// Linux 3 is EL8), so name the Enterprise Linux release.
			release = fmt.Sprint(major)
		} else {
			return repository{}, fmt.Errorf("Docker publishes no packages for %s", cmpOr(d.OS.PrettyName, d.OS.ID))
		}
		return repository{file: "etc/yum.repos.d/docker-ce.repo", content: fmt.Sprintf(`[docker-ce-stable]
name=Docker CE Stable - $basearch
baseurl=%[1]s/linux/%[2]s/%[3]s/$basearch/stable
enabled=1
gpgcheck=1
gpgkey=%[1]s/linux/%[2]s/gpg
`, base, distro, release)}, nil
	case d.Manager.Name() == "brew":
		return repository{}, errors.New("Docker Engine runs on Linux; on macOS install Docker Desktop or OrbStack (brew install --cask orbstack)")
	default: // apt
		distro := "debian"
		if d.OS.Is("ubuntu") {
			distro = "ubuntu"
		} else if !d.OS.Is("debian") {
			return repository{}, fmt.Errorf("Docker publishes no packages for %s", cmpOr(d.OS.PrettyName, d.OS.ID))
		}
		if d.OS.Codename == "" {
			return repository{}, errors.New("the release codename is missing from /etc/os-release")
		}
		key := "etc/apt/keyrings/docker.asc"
		return repository{
			file: "etc/apt/sources.list.d/docker.list",
			content: fmt.Sprintf("deb [signed-by=/%s] %s/linux/%s %s stable\n",
				key, base, distro, d.OS.Codename),
			keyURL: base + "/linux/" + distro + "/gpg", keyFile: key,
		}, nil
	}
}

// Install adds Docker's repository from the source with the given ID,
// installs the packages and starts the service.
func (d *Docker) Install(ctx context.Context, sourceID string, out func(string)) error {
	var source *DockerSource
	for i := range DockerSources {
		if DockerSources[i].ID == sourceID {
			source = &DockerSources[i]
		}
	}
	if source == nil {
		return fmt.Errorf("%w source %q", ErrInvalid, sourceID)
	}
	repo, err := d.repository(source.URL)
	if err != nil {
		return err
	}
	if repo.keyURL != "" {
		// apt fetches the repository over HTTPS, which needs the CA
		// certificates minimal images leave out.
		if err := d.Manager.Refresh(ctx, out); err != nil {
			return err
		}
		if err := d.Manager.Install(ctx, []string{"ca-certificates"}, out); err != nil {
			return err
		}
		out("Downloading the signing key from " + repo.keyURL)
		if err := d.download(ctx, repo.keyURL, repo.keyFile); err != nil {
			return err
		}
	}
	out("Adding the repository /" + repo.file)
	if err := d.write(repo.file, []byte(repo.content)); err != nil {
		return err
	}
	if d.Manager.Name() == "apt" {
		if err := d.Manager.Refresh(ctx, out); err != nil {
			return err
		}
	}
	if err := d.Manager.Install(ctx, dockerPackages, out); err != nil {
		return err
	}
	if _, err := os.Stat(filepath.Join(d.Root, "run", "systemd", "system")); err != nil {
		out("systemd is not running here: start dockerd yourself.")
		return nil
	}
	out("Starting Docker")
	return d.Run.Stream(ctx, nil, out, "systemctl", "enable", "--now", "docker")
}

func (d *Docker) write(name string, data []byte) error {
	path := filepath.Join(d.Root, name)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	return os.WriteFile(path, data, 0o644)
}

// download saves the key at url as name, trying a few times: links to
// overseas hosts often drop connections.
func (d *Docker) download(ctx context.Context, url, name string) error {
	var err error
	for attempt := range 3 {
		if attempt > 0 {
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-time.After(time.Duration(attempt) * 2 * time.Second):
			}
		}
		if err = d.fetch(ctx, url, name); err == nil {
			return nil
		}
	}
	return err
}

func (d *Docker) fetch(ctx context.Context, url, name string) error {
	client := d.HTTP
	if client == nil {
		client = &http.Client{Timeout: time.Minute}
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return err
	}
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("download %s: %s", url, resp.Status)
	}
	data, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return err
	}
	if !strings.Contains(string(data), "BEGIN PGP PUBLIC KEY BLOCK") {
		return fmt.Errorf("download %s: not a PGP public key", url)
	}
	return d.write(name, data)
}

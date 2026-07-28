package server

import (
	"archive/tar"
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/docker/docker/api/types"
	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/api/types/image"
	"github.com/docker/docker/api/types/network"
	"github.com/docker/docker/api/types/volume"
	"github.com/docker/docker/client"
	"github.com/docker/go-connections/nat"
	"github.com/lsongdev/ohmyserver/config"
	terminalpkg "github.com/lsongdev/ohmyserver/terminal"
)

type sse struct {
	w       http.ResponseWriter
	flusher http.Flusher
}

// DockerClient wraps the official Docker SDK client
type DockerClient struct {
	*client.Client
	registryMirror string
}

// NewDockerClient creates a new Docker client
func NewDockerClient(registryMirror string) (*DockerClient, error) {
	c, err := client.NewClientWithOpts(client.FromEnv)
	if err != nil {
		return nil, err
	}
	return &DockerClient{Client: c, registryMirror: registryMirror}, nil
}

// Container Operations

func (d *DockerClient) ListContainers() ([]types.Container, error) {
	ctx := context.Background()
	return d.Client.ContainerList(ctx, container.ListOptions{All: true})
}

func (d *DockerClient) GetContainer(id string) (types.ContainerJSON, error) {
	ctx := context.Background()
	return d.Client.ContainerInspect(ctx, id)
}

// GetContainerLogs retrieves logs for a container
func (d *DockerClient) GetContainerLogs(id string) (string, error) {
	ctx := context.Background()
	out, err := d.Client.ContainerLogs(ctx, id, types.ContainerLogsOptions{
		ShowStdout: true,
		ShowStderr: true,
	})
	if err != nil {
		return "", err
	}
	defer out.Close()

	data, err := io.ReadAll(out)
	if err != nil {
		return "", err
	}

	// Remove Docker log header (first 8 bytes)
	logs := string(data)
	return logs, nil
}

// StreamContainerLogs streams logs for a container in real-time
func (d *DockerClient) StreamContainerLogs(id string) (<-chan string, error) {
	ctx := context.Background()

	out, err := d.Client.ContainerLogs(ctx, id, types.ContainerLogsOptions{
		ShowStdout: true,
		ShowStderr: true,
		Follow:     true,
		Tail:       "100",
		Timestamps: false,
	})
	if err != nil {
		return nil, err
	}

	logCh := make(chan string, 100)

	go func() {
		defer out.Close()
		defer close(logCh)

		reader := bufio.NewReader(out)
		for {
			line, err := reader.ReadBytes('\n')
			if err != nil {
				if err == io.EOF {
					break
				}
				logCh <- fmt.Sprintf("Error reading log: %v", err)
				return
			}

			// Remove Docker log header (8 bytes: [8][0][0][0][0][0][0][0])
			if len(line) > 8 {
				line = line[8:]
			}

			if len(line) > 0 {
				logCh <- string(line)
			}
		}
	}()

	return logCh, nil
}

func (d *DockerClient) CreateContainer(imageName, containerName string, command []string) (string, error) {
	config := &container.Config{
		Image: imageName,
		Tty:   true,
	}
	if len(command) > 0 {
		config.Cmd = command
	}

	hostConfig := &container.HostConfig{}

	// Try to get exposed ports from image and bind them to host
	inspect, _, err := d.Client.ImageInspectWithRaw(context.Background(), imageName)
	if err == nil && inspect.Config != nil && len(inspect.Config.ExposedPorts) > 0 {
		config.ExposedPorts = inspect.Config.ExposedPorts
		hostConfig.PortBindings = make(nat.PortMap)
		for port := range inspect.Config.ExposedPorts {
			hostConfig.PortBindings[port] = []nat.PortBinding{
				{HostIP: "0.0.0.0"},
			}
			fmt.Printf("Binding exposed port: %s\n", port)
		}
	}

	resp, err := d.Client.ContainerCreate(context.Background(), config, hostConfig, nil, nil, containerName)
	if err != nil {
		return "", err
	}
	return resp.ID, nil
}

func (d *DockerClient) RemoveContainer(name string) error {
	ctx := context.Background()
	return d.Client.ContainerRemove(ctx, name, container.RemoveOptions{Force: true})
}

func (d *DockerClient) StartContainer(name string) error {
	ctx := context.Background()
	return d.Client.ContainerStart(ctx, name, container.StartOptions{})
}

func (d *DockerClient) StopContainer(name string) error {
	ctx := context.Background()
	timeout := 10
	return d.Client.ContainerStop(ctx, name, container.StopOptions{Timeout: &timeout})
}

func (d *DockerClient) RestartContainer(name string) error {
	ctx := context.Background()
	timeout := 10
	return d.Client.ContainerRestart(ctx, name, container.StopOptions{Timeout: &timeout})
}

// Image Operations

func (d *DockerClient) ListImages() ([]image.Summary, error) {
	ctx := context.Background()
	return d.Client.ImageList(ctx, types.ImageListOptions{})
}

func (d *DockerClient) ImageExists(name string) (bool, error) {
	ctx := context.Background()
	_, _, err := d.Client.ImageInspectWithRaw(ctx, name)
	if err != nil {
		if client.IsErrNotFound(err) {
			return false, nil
		}
		return false, err
	}
	return true, nil
}

func (d *DockerClient) PullImageSync(name string) error {
	ctx := context.Background()
	// Normalize image name: add library/ prefix for official images
	imageName := name
	if !strings.Contains(name, "/") {
		// Official Docker Hub image, add library/ prefix
		imageName = "library/" + name
	}
	reader, err := d.Client.ImagePull(ctx, imageName, types.ImagePullOptions{})
	if err == nil {
		defer reader.Close()
		_, err = io.Copy(io.Discard, reader)
		if err == nil {
			return nil
		}
	}
	return fmt.Errorf("failed to pull image: %w", err)
}

type ImagePullResponseItem struct {
	Status   string `json:"status"`
	Error    string `json:"error"`
	Progress string `json:"progress"`
}

func (d *DockerClient) PullImage(name string) (<-chan string, error) {
	ctx := context.Background()
	imageName := name
	if !strings.Contains(name, "/") {
		// Official Docker Hub image, add library/ prefix
		imageName = "library/" + name
	}
	reader, err := d.Client.ImagePull(ctx, imageName, types.ImagePullOptions{})
	if err != nil {
		return nil, err
	}
	logCh := make(chan string, 100)
	go func() {
		defer reader.Close()
		defer close(logCh)
		decoder := json.NewDecoder(reader)
		for {
			var event ImagePullResponseItem
			if err := decoder.Decode(&event); err != nil {
				if err == io.EOF {
					break
				}
				logCh <- fmt.Sprintf("Error: %v", err)
				return
			}
			// log.Println(event)
			if event.Status != "" {
				if event.Progress != "" {
					logCh <- event.Status + ": " + event.Progress
				} else {
					logCh <- event.Status
				}
			} else if event.Error != "" {
				logCh <- fmt.Sprintf("Error: %s", event.Error)
				return
			}
		}
	}()
	return logCh, nil
}

func (d *DockerClient) RemoveImage(imageId string) error {
	ctx := context.Background()
	_, err := d.Client.ImageRemove(ctx, imageId, types.ImageRemoveOptions{Force: true})
	return err
}

func (d *DockerClient) BuildImage(name, dockerfile string) (<-chan string, error) {
	ctx := context.Background()
	buildContext := io.NopCloser(strings.NewReader(dockerfile))
	resp, err := d.Client.ImageBuild(ctx, buildContext, types.ImageBuildOptions{
		Tags:       []string{name},
		Dockerfile: "Dockerfile",
		Remove:     true,
	})
	if err != nil {
		return nil, err
	}

	logCh := make(chan string, 100)
	go func() {
		defer resp.Body.Close()
		defer close(logCh)

		decoder := json.NewDecoder(resp.Body)
		for {
			var msg struct {
				Stream      string `json:"stream"`
				Error       string `json:"error"`
				ErrorDetail struct {
					Message string `json:"message"`
				} `json:"errorDetail"`
			}

			if err := decoder.Decode(&msg); err != nil {
				if err == io.EOF {
					break
				}
				logCh <- fmt.Sprintf("Error: %v", err)
				return
			}

			if msg.Stream != "" {
				logCh <- strings.TrimSuffix(msg.Stream, "\n")
			}
			if msg.Error != "" {
				logCh <- fmt.Sprintf("Error: %s", msg.Error)
				return
			}
			if msg.ErrorDetail.Message != "" {
				logCh <- fmt.Sprintf("Error: %s", msg.ErrorDetail.Message)
				return
			}
		}
		logCh <- "Image build completed"
	}()

	return logCh, nil
}

// BuildImageFromDir builds a Docker image from a directory containing a Dockerfile
func (d *DockerClient) BuildImageFromDir(name, buildDir string) (<-chan string, error) {
	ctx := context.Background()

	fmt.Printf("Building image '%s' from directory: %s\n", name, buildDir)

	// Create a tar archive of the build directory
	tarReader, err := createTarFromDir(buildDir)
	if err != nil {
		return nil, fmt.Errorf("failed to create tar archive: %w", err)
	}
	defer tarReader.Close()

	resp, err := d.Client.ImageBuild(ctx, tarReader, types.ImageBuildOptions{
		Tags:       []string{name},
		Dockerfile: "Dockerfile",
		Remove:     true,
		NoCache:    true,
	})
	if err != nil {
		return nil, fmt.Errorf("image build failed: %w", err)
	}

	logCh := make(chan string, 100)
	go func() {
		defer resp.Body.Close()
		defer close(logCh)

		// Read and log build output line by line
		decoder := json.NewDecoder(resp.Body)
		for {
			var msg struct {
				Stream      string `json:"stream"`
				Error       string `json:"error"`
				ErrorDetail struct {
					Message string `json:"message"`
				} `json:"errorDetail"`
				Status   string `json:"status"`
				Progress string `json:"progress"`
				ID       string `json:"id"`
				From     string `json:"from"`
			}

			if err := decoder.Decode(&msg); err != nil {
				if err == io.EOF {
					break
				}
				logCh <- fmt.Sprintf("Error: %v", err)
				return
			}

			// Print build output
			if msg.Stream != "" {
				fmt.Print(msg.Stream)
				logCh <- strings.TrimSuffix(msg.Stream, "\n")
			}
			if msg.Status != "" {
				var logLine string
				if msg.Progress != "" {
					logLine = fmt.Sprintf("%s: %s", msg.Status, msg.Progress)
				} else {
					logLine = msg.Status
				}
				fmt.Printf("%s\n", logLine)
				logCh <- logLine
			}
			if msg.ID != "" && msg.From != "" {
				logLine := fmt.Sprintf("Step: %s from %s", msg.ID, msg.From)
				fmt.Printf("%s\n", logLine)
				logCh <- logLine
			}

			// Check for error
			if msg.Error != "" {
				logCh <- fmt.Sprintf("Error: %s", msg.Error)
				return
			}
			if msg.ErrorDetail.Message != "" {
				logCh <- fmt.Sprintf("Error: %s", msg.ErrorDetail.Message)
				return
			}
		}

		// Verify image was created
		fmt.Printf("\nVerifying image '%s' was created...\n", name)
		inspect, _, err := d.Client.ImageInspectWithRaw(ctx, name)
		if err != nil {
			logCh <- fmt.Sprintf("Warning: Image '%s' not found after build: %v", name, err)
		} else {
			logCh <- fmt.Sprintf("Image '%s' built successfully, ID: %s", name, inspect.ID[:12])
		}
	}()

	return logCh, nil
}

// BuildImageFromDirContext builds an image synchronously, propagating both
// cancellation and errors from Docker's streamed build response.
func (d *DockerClient) BuildImageFromDirContext(ctx context.Context, name, buildDir string, onLog func(string)) error {
	tarReader, err := createTarFromDir(buildDir)
	if err != nil {
		return fmt.Errorf("failed to create tar archive: %w", err)
	}
	defer tarReader.Close()

	resp, err := d.Client.ImageBuild(ctx, tarReader, types.ImageBuildOptions{
		Tags:       []string{name},
		Dockerfile: "Dockerfile",
		Remove:     true,
		NoCache:    true,
	})
	if err != nil {
		return fmt.Errorf("image build failed: %w", err)
	}
	defer resp.Body.Close()

	decoder := json.NewDecoder(resp.Body)
	for {
		var msg struct {
			Stream      string `json:"stream"`
			Error       string `json:"error"`
			ErrorDetail struct {
				Message string `json:"message"`
			} `json:"errorDetail"`
			Status   string `json:"status"`
			Progress string `json:"progress"`
		}
		if err := decoder.Decode(&msg); err != nil {
			if err == io.EOF {
				return nil
			}
			return fmt.Errorf("decode image build response: %w", err)
		}
		if msg.Error != "" {
			return fmt.Errorf("docker build: %s", msg.Error)
		}
		if msg.ErrorDetail.Message != "" {
			return fmt.Errorf("docker build: %s", msg.ErrorDetail.Message)
		}
		if onLog == nil {
			continue
		}
		if line := strings.TrimSpace(msg.Stream); line != "" {
			onLog(line)
		}
		if msg.Status != "" {
			line := msg.Status
			if msg.Progress != "" {
				line += ": " + msg.Progress
			}
			onLog(line)
		}
	}
}

// createTarFromDir creates a tar archive reader from a directory
func createTarFromDir(srcDir string) (io.ReadCloser, error) {
	pr, pw := io.Pipe()
	go func() {
		defer pw.Close()
		tw := tar.NewWriter(pw)
		defer tw.Close()

		fmt.Printf("Creating tar archive from: %s\n", srcDir)
		fileCount := 0

		err := filepath.Walk(srcDir, func(path string, info os.FileInfo, err error) error {
			if err != nil {
				return err
			}

			// Get relative path
			relPath, err := filepath.Rel(srcDir, path)
			if err != nil {
				return err
			}

			// Skip the root directory itself
			if relPath == "." {
				return nil
			}

			// Skip .git directory
			if info.IsDir() && info.Name() == ".git" {
				fmt.Printf("Skipping .git directory\n")
				return filepath.SkipDir
			}

			// Create tar header
			header, err := tar.FileInfoHeader(info, info.Name())
			if err != nil {
				return err
			}
			header.Name = relPath

			if err := tw.WriteHeader(header); err != nil {
				return err
			}

			// If it's a file, write its content
			if !info.IsDir() {
				fileData, err := os.Open(path)
				if err != nil {
					return err
				}
				defer fileData.Close()
				if _, err := io.Copy(tw, fileData); err != nil {
					return err
				}
				fileCount++
				fmt.Printf("Added to tar: %s\n", relPath)
			}

			return nil
		})

		fmt.Printf("Total files added to tar: %d\n", fileCount)

		if err != nil {
			pw.CloseWithError(err)
		}
	}()
	return pr, nil
}

// BuildImageFromRepo clones a git repository and builds a Docker image from it
func (d *DockerClient) BuildImageFromRepo(name, repoURL, branch string) (<-chan string, error) {
	// Create a temporary directory for the repository
	tmpDir, err := os.MkdirTemp("", "docker-build-repo-*")
	if err != nil {
		return nil, fmt.Errorf("failed to create temp directory: %w", err)
	}
	defer os.RemoveAll(tmpDir)

	logCh := make(chan string, 100)

	go func() {
		defer close(logCh)

		logCh <- fmt.Sprintf("Building image '%s' from repository: %s", name, repoURL)
		logCh <- fmt.Sprintf("Cloning repository to: %s", tmpDir)

		// Clone the repository
		cloneArgs := []string{"clone", "--depth=1"}
		if branch != "" {
			cloneArgs = append(cloneArgs, "--branch", branch)
		}
		cloneArgs = append(cloneArgs, repoURL, tmpDir)

		cmd := exec.Command("git", cloneArgs...)
		output, err := cmd.CombinedOutput()
		if err != nil {
			logCh <- fmt.Sprintf("Error: Failed to clone repository: %v", err)
			logCh <- fmt.Sprintf("Output: %s", string(output))
			return
		}

		logCh <- "Repository cloned successfully"

		// Build image from the cloned repository
		buildLogCh, err := d.BuildImageFromDir(name, tmpDir)
		if err != nil {
			logCh <- fmt.Sprintf("Error: %v", err)
			return
		}

		// Forward build logs
		for log := range buildLogCh {
			logCh <- log
		}
	}()

	return logCh, nil
}

// Volume Operations

func (d *DockerClient) ListVolumes() (volume.ListResponse, error) {
	ctx := context.Background()
	return d.Client.VolumeList(ctx, volume.ListOptions{})
}

func (d *DockerClient) CreateVolume(name, driver string) error {
	ctx := context.Background()
	_, err := d.Client.VolumeCreate(ctx, volume.CreateOptions{
		Name:   name,
		Driver: driver,
	})
	return err
}

func (d *DockerClient) RemoveVolume(name string) error {
	ctx := context.Background()
	return d.Client.VolumeRemove(ctx, name, true)
}

// Network Operations

func (d *DockerClient) ListNetworks() ([]types.NetworkResource, error) {
	ctx := context.Background()
	return d.Client.NetworkList(ctx, types.NetworkListOptions{})
}

func (d *DockerClient) CreateNetwork(name, driver, subnet, gateway, ipRange string) error {
	ctx := context.Background()
	_, err := d.Client.NetworkCreate(ctx, name, types.NetworkCreate{
		Driver: driver,
		IPAM: &network.IPAM{
			Driver: "default",
			Config: []network.IPAMConfig{{
				Subnet:  subnet,
				Gateway: gateway,
				IPRange: ipRange,
			}},
		},
	})
	return err
}

func (d *DockerClient) RemoveNetwork(id string) error {
	ctx := context.Background()
	return d.Client.NetworkRemove(ctx, id)
}

// ContainerTerminalView renders the shared terminal UI for a container.
func (s *Server) ContainerTerminalView(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if id == "" {
		http.Error(w, "container ID required", http.StatusBadRequest)
		return
	}
	s.renderTerminal(w, "Container Terminal", "/docker/containers/"+id+"/terminal/ws", "")
}

// ContainerTerminal bridges a Docker exec session through the terminal module.
func (s *Server) ContainerTerminal(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if id == "" {
		http.Error(w, "container ID required", http.StatusBadRequest)
		return
	}
	s.serveTerminal(w, r, terminalpkg.BackendFunc(func(ctx context.Context, cols, rows uint16) (terminalpkg.Session, error) {
		return s.openContainerTerminal(ctx, id, cols, rows)
	}))
}

func (s *Server) openContainerTerminal(
	ctx context.Context,
	containerID string,
	cols, rows uint16,
) (terminalpkg.Session, error) {
	execConfig := types.ExecConfig{
		Cmd:          []string{"/bin/sh"},
		Tty:          true,
		AttachStdin:  true,
		AttachStdout: true,
		AttachStderr: true,
	}

	execResp, err := s.docker.Client.ContainerExecCreate(ctx, containerID, execConfig)
	if err != nil {
		return nil, err
	}

	hijackResp, err := s.docker.Client.ContainerExecAttach(ctx, execResp.ID, types.ExecStartCheck{
		Tty:    true,
		Detach: false,
	})
	if err != nil {
		return nil, err
	}
	sessionCtx, cancel := context.WithCancel(ctx)
	result := &dockerTerminalSession{
		client: s.docker.Client, execID: execResp.ID,
		response: hijackResp, ctx: sessionCtx, cancel: cancel,
	}
	if err := result.Resize(cols, rows); err != nil {
		_ = result.Close()
		return nil, err
	}
	return result, nil
}

type dockerTerminalSession struct {
	client   *client.Client
	execID   string
	response types.HijackedResponse
	ctx      context.Context
	cancel   context.CancelFunc
}

func (s *dockerTerminalSession) Input() io.WriteCloser { return s.response.Conn }
func (s *dockerTerminalSession) Output() io.Reader     { return s.response.Reader }

func (s *dockerTerminalSession) Resize(cols, rows uint16) error {
	return s.client.ContainerExecResize(s.ctx, s.execID, container.ResizeOptions{
		Width: uint(cols), Height: uint(rows),
	})
}

func (s *dockerTerminalSession) Wait() error {
	ticker := time.NewTicker(250 * time.Millisecond)
	defer ticker.Stop()
	for {
		select {
		case <-s.ctx.Done():
			return s.ctx.Err()
		case <-ticker.C:
			info, err := s.client.ContainerExecInspect(s.ctx, s.execID)
			if err != nil {
				return err
			}
			if !info.Running {
				return nil
			}
		}
	}
}

func (s *dockerTerminalSession) Close() error {
	s.cancel()
	s.response.Close()
	return nil
}

func (s *Server) ContainerView(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case "GET":
		s.ListContainers(w, r)
	case "POST":
		s.CreateContainer(w, r)
	}
}

func (s *Server) ListContainers(w http.ResponseWriter, r *http.Request) {
	id := r.URL.Query().Get("id")
	if id != "" {
		container, err := s.docker.GetContainer(id)
		if err != nil {
			s.Error(w, err)
			return
		}
		s.Render(w, "docker/container_detail", H{
			"container": container,
		})
		return
	}
	containers, err := s.docker.ListContainers()
	if err != nil {
		s.Error(w, err)
		return
	}
	s.Render(w, "docker/containers", H{
		"containers": containers,
	})
}

// ContainerDetailView handles container detail page
func (s *Server) ContainerDetailView(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	container, err := s.docker.GetContainer(id)
	if err != nil {
		s.Error(w, err)
		return
	}
	s.Render(w, "docker/container_detail", H{
		"container": container,
	})
}

// ContainerActionHandler handles container actions (GET for detail, PUT for actions)
func (s *Server) ContainerActionHandler(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")

	switch r.Method {
	case "GET":
		container, err := s.docker.GetContainer(id)
		if err != nil {
			s.Error(w, err)
			return
		}
		s.Render(w, "docker/container_detail", H{
			"container": container,
		})

	case "PUT":
		action := r.URL.Query().Get("action")
		var err error
		switch action {
		case "start":
			err = s.docker.StartContainer(id)
		case "stop":
			err = s.docker.StopContainer(id)
		case "restart":
			err = s.docker.RestartContainer(id)
		default:
			http.Error(w, "Invalid action", http.StatusBadRequest)
			return
		}
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		w.WriteHeader(http.StatusOK)

	case "DELETE":
		if err := s.docker.RemoveContainer(id); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		w.WriteHeader(http.StatusOK)

	default:
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
	}
}

// ContainerStatsHandler handles container stats API
func (s *Server) ContainerStatsHandler(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")

	sse, err := NewSSE(w)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	// Send stats every 2 seconds
	ticker := time.NewTicker(2 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-r.Context().Done():
			return
		case <-ticker.C:
			stats, err := s.docker.GetContainerStats(id)
			if err != nil {
				sse.Send("error", err.Error())
				continue
			}

			statsJSON, err := json.Marshal(stats)
			if err != nil {
				sse.Send("error", err.Error())
				continue
			}

			sse.Send("stats", string(statsJSON))
		}
	}
}

// GetContainerStats gets real-time statistics for a container
func (d *DockerClient) GetContainerStats(containerID string) (*ContainerStats, error) {
	ctx := context.Background()

	resp, err := d.Client.ContainerStats(ctx, containerID, false)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	var stats types.StatsJSON
	if err := json.NewDecoder(resp.Body).Decode(&stats); err != nil {
		return nil, err
	}

	// Calculate CPU percentage
	cpuPercent := calculateCPUPercent(stats)

	// Calculate memory usage and percentage
	memUsage := stats.MemoryStats.Usage
	memLimit := stats.MemoryStats.Limit
	memPercent := float64(0)
	if memLimit > 0 {
		memPercent = float64(memUsage) / float64(memLimit) * 100.0
	}

	// Calculate network I/O (sum all networks)
	var networkRx, networkTx uint64
	for _, net := range stats.Networks {
		networkRx += net.RxBytes
		networkTx += net.TxBytes
	}

	return &ContainerStats{
		CPUPercent:    cpuPercent,
		MemoryUsage:   memUsage,
		MemoryLimit:   memLimit,
		MemoryPercent: memPercent,
		NetworkRx:     networkRx,
		NetworkTx:     networkTx,
	}, nil
}

// calculateCPUPercent calculates CPU usage percentage
func calculateCPUPercent(stats types.StatsJSON) float64 {
	cpuDelta := float64(stats.CPUStats.CPUUsage.TotalUsage) - float64(stats.PreCPUStats.CPUUsage.TotalUsage)
	systemDelta := float64(stats.CPUStats.SystemUsage) - float64(stats.PreCPUStats.SystemUsage)

	if systemDelta <= 0 || cpuDelta <= 0 {
		return 0
	}

	cpuCount := float64(len(stats.CPUStats.CPUUsage.PercpuUsage))
	if cpuCount == 0 {
		cpuCount = 1
	}

	return (cpuDelta / systemDelta) * cpuCount * 100.0
}

// ContainerStats represents container statistics
type ContainerStats struct {
	CPUPercent    float64 `json:"cpu_percent"`
	MemoryUsage   uint64  `json:"memory_usage"`
	MemoryLimit   uint64  `json:"memory_limit"`
	MemoryPercent float64 `json:"memory_percent"`
	NetworkRx     uint64  `json:"network_rx"`
	NetworkTx     uint64  `json:"network_tx"`
}

// ContainerLogsHandler handles container logs API with SSE streaming
func (s *Server) ContainerLogsHandler(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")

	// Check if client wants SSE stream
	if r.Header.Get("Accept") == "text/event-stream" {
		sse, err := NewSSE(w)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		logCh, err := s.docker.StreamContainerLogs(id)
		if err != nil {
			sse.Send("error", err.Error())
			return
		}

		// Send logs as SSE events
		for log := range logCh {
			sse.Send("log", log)
		}

		sse.Send("connected", "Log stream ended")
		return
	}

	// Otherwise return all logs at once
	logs, err := s.docker.GetContainerLogs(id)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "text/plain")
	w.Write([]byte(logs))
}

// ImageDetailView handles image detail page
func (s *Server) ImageDetailView(w http.ResponseWriter, r *http.Request) {
	images, err := s.docker.ListImages()
	if err != nil {
		s.Error(w, err)
		return
	}
	s.Render(w, "docker/images", H{
		"images": images,
	})
}

// VolumeDetailView handles volume detail page
func (s *Server) VolumeDetailView(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	volumes, err := s.docker.ListVolumes()
	if err != nil {
		s.Error(w, err)
		return
	}
	// Filter volumes by name
	var filteredVolumes []*volume.Volume
	for _, vol := range volumes.Volumes {
		if vol.Name == name {
			filteredVolumes = append(filteredVolumes, vol)
		}
	}
	s.Render(w, "docker/volumes", H{
		"volumes": filteredVolumes,
	})
}

// NetworkDetailView handles network detail page
func (s *Server) NetworkDetailView(w http.ResponseWriter, r *http.Request) {
	// id := r.PathValue("id")
	networks, err := s.docker.ListNetworks()
	if err != nil {
		s.Error(w, err)
		return
	}
	s.Render(w, "docker/networks", H{
		"networks": networks,
	})
}

func (s *Server) CreateContainer(w http.ResponseWriter, r *http.Request) {
	if r.Method == "GET" {
		images, err := s.docker.ListImages()
		if err != nil {
			s.Error(w, err)
			return
		}
		imageNames := make([]string, 0)
		for _, img := range images {
			imageNames = append(imageNames, img.RepoTags...)
		}
		s.Render(w, "docker/container_new", H{
			"imageNames": imageNames,
		})
		return
	}
	name := r.FormValue("name")
	imageName := r.FormValue("image")
	commandStr := r.FormValue("command")

	var command []string
	if commandStr != "" {
		command = []string{"/bin/sh", "-c", commandStr}
	}

	containerId, err := s.docker.CreateContainer(imageName, name, command)
	if err != nil {
		s.Error(w, err)
		return
	}
	http.Redirect(w, r, fmt.Sprintf("/docker/containers/%s", containerId), http.StatusSeeOther)
}

func (s *Server) ImageView(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case "GET":
		if r.URL.Query().Get("name") != "" {
			s.PullImage(w, r)
			return
		}
		s.ListImages(w, r)
	case "POST":
		if r.URL.Query().Get("action") == "build" {
			s.BuildImage(w, r)
		} else {
			s.PullImage(w, r)
		}
	case "DELETE":
		s.RemoveImage(w, r)
	}
}

func (s *Server) ListImages(w http.ResponseWriter, r *http.Request) {
	images, err := s.docker.ListImages()
	if err != nil {
		s.Error(w, err)
		return
	}
	s.Render(w, "docker/images", H{
		"images": images,
	})
}

func (s *Server) PullImage(w http.ResponseWriter, r *http.Request) {
	name := r.URL.Query().Get("name")
	if name == "" {
		name = r.FormValue("name")
	}

	if r.Method == "GET" && name == "" {
		s.Render(w, "docker/image_pull", H{})
		return
	}

	sse, err := NewSSE(w)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	logCh, err := s.docker.PullImage(name)
	if err != nil {
		sse.Send("error", err.Error())
		return
	}

	// Send logs as SSE events
	for log := range logCh {
		sse.Send("log", log)
	}

	// Send completion event
	sse.Send("done", "Image pull completed")
}

func (s *Server) BuildImage(w http.ResponseWriter, r *http.Request) {
	if r.Method == "GET" {
		s.Render(w, "docker/image_build", H{})
		return
	}

	name := r.FormValue("name")
	sourceType := r.FormValue("source_type")

	if name == "" {
		http.Error(w, "Image name is required", http.StatusBadRequest)
		return
	}

	// Use SSE for real-time build logs
	sse, err := NewSSE(w)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	var logCh <-chan string
	switch sourceType {
	case "directory":
		buildDir := r.FormValue("build_dir")
		if buildDir == "" {
			sse.Send("error", "Build directory is required")
			return
		}
		logCh, err = s.docker.BuildImageFromDir(name, buildDir)
	case "repository":
		repoURL := r.FormValue("repo_url")
		branch := r.FormValue("branch")
		if repoURL == "" {
			sse.Send("error", "Repository URL is required")
			return
		}
		logCh, err = s.docker.BuildImageFromRepo(name, repoURL, branch)
	default:
		// Default to Dockerfile content
		dockerfile := r.FormValue("dockerfile")
		if dockerfile == "" {
			sse.Send("error", "Dockerfile is required")
			return
		}
		logCh, err = s.docker.BuildImage(name, dockerfile)
	}

	if err != nil {
		sse.Send("error", fmt.Sprintf("Failed to build image: %v", err))
		return
	}

	// Send logs as SSE events
	for log := range logCh {
		// Escape data for SSE
		escapedLog := strings.ReplaceAll(log, "\n", "\\n")
		sse.Send("log", escapedLog)
	}

	// Send completion event
	sse.Send("done", "Image build completed")
}

func (s *Server) RemoveImage(w http.ResponseWriter, r *http.Request) {
	id := r.URL.Query().Get("id")
	if err := s.docker.RemoveImage(id); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.WriteHeader(http.StatusOK)
}

func (s *Server) VolumeView(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case "GET":
		s.ListVolumes(w, r)
	case "POST":
		s.CreateVolume(w, r)
	case "DELETE":
		s.RemoveVolume(w, r)
	}
}

func (s *Server) ListVolumes(w http.ResponseWriter, r *http.Request) {
	volumes, err := s.docker.ListVolumes()
	if err != nil {
		s.Error(w, err)
		return
	}
	s.Render(w, "docker/volumes", H{
		"volumes": volumes.Volumes,
	})
}

func (s *Server) CreateVolume(w http.ResponseWriter, r *http.Request) {
	if r.Method == "GET" {
		s.Render(w, "docker/create_volume", H{})
		return
	}
	name := r.FormValue("name")
	driver := r.FormValue("driver")
	if err := s.docker.CreateVolume(name, driver); err != nil {
		s.Error(w, err)
		return
	}
	s.ListVolumes(w, r)
}

func (s *Server) RemoveVolume(w http.ResponseWriter, r *http.Request) {
	name := r.URL.Query().Get("id")
	if err := s.docker.RemoveVolume(name); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.WriteHeader(http.StatusOK)
}

func (s *Server) NetworkView(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case "GET":
		s.ListNetworks(w, r)
	case "POST":
		s.CreateNetwork(w, r)
	case "DELETE":
		s.RemoveNetwork(w, r)
	}
}

func (s *Server) ListNetworks(w http.ResponseWriter, r *http.Request) {
	networks, err := s.docker.ListNetworks()
	if err != nil {
		s.Error(w, err)
		return
	}
	s.Render(w, "docker/networks", H{
		"networks": networks,
	})
}

func (s *Server) CreateNetwork(w http.ResponseWriter, r *http.Request) {
	if r.Method == "GET" {
		s.Render(w, "docker/create_network", H{})
		return
	}

	name := r.FormValue("name")
	driver := r.FormValue("driver")
	subnet := r.FormValue("subnet")
	gateway := r.FormValue("gateway")
	ipRange := r.FormValue("iprange")
	if err := s.docker.CreateNetwork(name, driver, subnet, gateway, ipRange); err != nil {
		s.Error(w, err)
		return
	}
	s.ListNetworks(w, r)
}

func (s *Server) RemoveNetwork(w http.ResponseWriter, r *http.Request) {
	id := r.URL.Query().Get("id")
	if err := s.docker.RemoveNetwork(id); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.WriteHeader(http.StatusOK)
}

// ComposeProjectMetadata stores project configuration
type ComposeProject struct {
	Name        string    `json:"Name"`
	Status      string    `json:"Status"`
	ConfigFiles string    `json:"ConfigFiles"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
}

// ComposeView lists all compose projects
func (s *Server) ComposeView(w http.ResponseWriter, r *http.Request) {
	projects, err := s.ListComposeProjects()
	if err != nil {
		s.Error(w, err)
		return
	}
	s.Render(w, "docker/compose", H{
		"projects": projects,
	})
}

// ComposeDetailView shows detail for a specific compose project
func (s *Server) ComposeDetailView(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	if name == "" {
		s.Error(w, &Error{Message: "Project name is required"})
		return
	}

	project, err := s.GetComposeProjectDetail(name)
	if err != nil {
		s.Error(w, err)
		return
	}

	containers := s.getComposeContainers(name)

	s.Render(w, "docker/compose_detail", H{
		"project":    project,
		"containers": containers,
	})
}

// ComposeNewHandler shows the new compose form and handles creation
func (s *Server) ComposeNewHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method == "GET" {
		s.Render(w, "docker/compose_new", H{})
		return
	}
}

func findComposeFile(dir string) string {
	candidates := []string{
		"docker-compose.yml",
		"docker-compose.yaml",
		"compose.yml",
		"compose.yaml",
	}
	for _, candidate := range candidates {
		path := filepath.Join(dir, candidate)
		if _, err := os.Stat(path); err == nil {
			return path
		}
	}
	return ""
}

// ComposeActionHandler handles compose actions (up, down, restart, logs)
func (s *Server) ComposeActionHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	action := r.FormValue("action")
	projectName := r.FormValue("project_name")
	configFile := r.FormValue("config_file")

	if projectName == "" || configFile == "" {
		s.Error(w, &Error{Message: "project_name and config_file are required"})
		return
	}

	var err error
	switch action {
	case "up":
		err = s.ComposeUp(configFile)
	case "down":
		err = s.ComposeDown(configFile)
	case "restart":
		err = s.ComposeRestart(configFile)
	case "pull":
		err = s.ComposePull(configFile)
	default:
		s.Error(w, &Error{Message: "Invalid action: " + action})
		return
	}

	if err != nil {
		s.Error(w, err)
		return
	}

	http.Redirect(w, r, "/docker/compose", http.StatusSeeOther)
}

// Compose Up handler with SSE log streaming
func (s *Server) ComposeUpSSEHandler(w http.ResponseWriter, r *http.Request) {
	if err := ParseFormAll(r); err != nil {
		http.Error(w, "Failed to parse form: "+err.Error(), http.StatusBadRequest)
		return
	}

	// Use SSE for real-time logs
	sse, err := NewSSE(w)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	name := r.FormValue("name")
	dir := r.FormValue("dir")
	repo := r.FormValue("repo")
	branch := r.FormValue("branch")
	composeContent := r.FormValue("compose")

	log.Printf("ComposeUp form data - name: %q, dir: %q, repo: %q, branch: %q", name, dir, repo, branch)

	if dir == "" {
		dir = filepath.Join(config.DockerComposeDir, name)
	}
	if repo != "" {
		cloneCmd := exec.Command("git", "clone", "--depth", "1", repo, dir)
		if branch != "" {
			cloneCmd = exec.Command("git", "clone", "--depth", "1", "--branch", branch, repo, dir)
		}
		if output, err := cloneCmd.CombinedOutput(); err != nil {
			sse.Send("error", fmt.Sprintf("Failed to clone repository: %s", string(output)))
			return
		}
	}

	var configFile string
	if composeContent != "" {
		configFile = filepath.Join(dir, "docker-compose.yaml")
		if err := os.WriteFile(configFile, []byte(composeContent), 0644); err != nil {
			sse.Send("error", fmt.Sprintf("Failed to write compose file: %v", err))
			return
		}
	}

	configFile = findComposeFile(dir)
	// Send connected event
	sse.Send("connected", "Starting compose up...")
	// Run docker-compose up with log streaming
	cmd := exec.Command("docker-compose", "-f", configFile, "up", "-d")
	cmd.Dir = filepath.Dir(configFile)

	stdout, err := cmd.StdoutPipe()
	if err != nil {
		sse.Send("error", fmt.Sprintf("Failed to create stdout pipe: %s", err.Error()))
		return
	}

	stderr, err := cmd.StderrPipe()
	if err != nil {
		sse.Send("error", fmt.Sprintf("Failed to create stderr pipe: %s", err.Error()))
		return
	}

	if err := cmd.Start(); err != nil {
		sse.Send("error", fmt.Sprintf("Failed to start command: %s", err.Error()))
		return
	}

	// Stream stdout
	done := make(chan bool, 1)
	go func() {
		scanner := bufio.NewScanner(stdout)
		for scanner.Scan() {
			line := scanner.Text()
			sse.Send("log", line)
		}
		done <- true
	}()

	// Stream stderr
	go func() {
		scanner := bufio.NewScanner(stderr)
		for scanner.Scan() {
			line := scanner.Text()
			sse.Send("log", line)
		}
	}()

	// Wait for command to finish
	err = cmd.Wait()
	<-done

	if err != nil {
		sse.Send("error", fmt.Sprintf("Compose up failed: %s", err.Error()))
	} else {
		sse.Send("complete", "Compose up completed successfully")
	}
}

// Compose Operations

func (s *Server) ListComposeProjects() (projects []ComposeProject, err error) {
	// Use docker-compose ls to get projects
	cmd := exec.Command("docker-compose", "ls", "--format", "json", "--all")
	output, err := cmd.CombinedOutput()
	if err != nil {
		// If command fails, return empty list
		return projects, nil
	}
	if err = json.Unmarshal(output, &projects); err != nil {
		return
	}
	return projects, nil
}

// GetComposeProjectDetail gets detailed information for a specific compose project
func (s *Server) GetComposeProjectDetail(name string) (*ComposeProject, error) {
	projects, err := s.ListComposeProjects()
	if err != nil {
		return nil, err
	}

	for _, project := range projects {
		if project.Name == name {
			return &project, nil
		}
	}

	return nil, &Error{Message: "Compose project not found: " + name}
}

// getComposeContainers gets running containers for a compose project
func (s *Server) getComposeContainers(name string) (containers []map[string]any) {
	containers = make([]map[string]any, 0)
	cmd := exec.Command("docker-compose", "-p", name, "ps", "--format", "json")
	output, err := cmd.CombinedOutput()
	if err != nil {
		return containers
	}
	// Parse JSONL (JSON Lines) - each line is a separate JSON object
	lines := strings.Split(strings.TrimSpace(string(output)), "\n")
	for _, line := range lines {
		if line == "" {
			continue
		}
		var container map[string]any
		if err := json.Unmarshal([]byte(line), &container); err != nil {
			continue
		}
		containers = append(containers, container)
	}
	return containers
}

func (s *Server) ComposeUp(configFile string) error {
	dir := filepath.Dir(configFile)
	cmd := exec.Command("docker-compose", "-f", configFile, "up", "-d")
	cmd.Dir = dir
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("compose up failed: %s: %w", string(output), err)
	}
	return nil
}

func (s *Server) ComposeDown(configFile string) error {
	dir := filepath.Dir(configFile)
	cmd := exec.Command("docker-compose", "-f", configFile, "down")
	cmd.Dir = dir
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("compose down failed: %s: %w", string(output), err)
	}
	return nil
}

func (s *Server) ComposeRestart(configFile string) error {
	dir := filepath.Dir(configFile)
	cmd := exec.Command("docker-compose", "-f", configFile, "restart")
	cmd.Dir = dir
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("compose restart failed: %s: %w", string(output), err)
	}
	return nil
}

func (s *Server) ComposePull(configFile string) error {
	dir := filepath.Dir(configFile)
	cmd := exec.Command("docker-compose", "-f", configFile, "pull")
	cmd.Dir = dir
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("compose pull failed: %s: %w", string(output), err)
	}
	return nil
}

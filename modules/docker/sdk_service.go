package docker

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"strings"

	"github.com/docker/docker/api/types"
	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/api/types/image"
	"github.com/docker/docker/api/types/network"
	"github.com/docker/docker/api/types/volume"
	"github.com/docker/docker/pkg/stdcopy"
	"github.com/docker/go-connections/nat"
)

// SDKService owns Docker Engine protocol calls for containers and resources.
// The module's LocalService composes it with Compose and catalog behavior.
type SDKService struct{ client *DockerClient }

func NewSDKService(client *DockerClient) *SDKService { return &SDKService{client: client} }
func (s *SDKService) ListDockerContainers(ctx context.Context) ([]types.Container, error) {
	return s.client.Client.ContainerList(ctx, container.ListOptions{All: true})
}
func (s *SDKService) InspectDockerContainer(ctx context.Context, id string) (types.ContainerJSON, error) {
	return s.client.Client.ContainerInspect(ctx, id)
}
func (s *SDKService) GetDockerContainerLogs(ctx context.Context, id string) (string, error) {
	info, err := s.client.Client.ContainerInspect(ctx, id)
	if err != nil {
		return "", err
	}
	reader, err := s.client.Client.ContainerLogs(ctx, id, types.ContainerLogsOptions{ShowStdout: true, ShowStderr: true, Tail: "200"})
	if err != nil {
		return "", err
	}
	defer reader.Close()
	if info.Config != nil && info.Config.Tty {
		var output bytes.Buffer
		_, err = io.Copy(&output, reader)
		return output.String(), err
	}
	var stdout, stderr bytes.Buffer
	if _, err = stdcopy.StdCopy(&stdout, &stderr, reader); err != nil {
		return "", err
	}
	return stdout.String() + stderr.String(), nil
}
func (s *SDKService) StartDockerContainer(ctx context.Context, id string) error {
	return s.client.Client.ContainerStart(ctx, id, container.StartOptions{})
}
func (s *SDKService) StopDockerContainer(ctx context.Context, id string) error {
	timeout := 10
	return s.client.Client.ContainerStop(ctx, id, container.StopOptions{Timeout: &timeout})
}
func (s *SDKService) RestartDockerContainer(ctx context.Context, id string) error {
	timeout := 10
	return s.client.Client.ContainerRestart(ctx, id, container.StopOptions{Timeout: &timeout})
}
func (s *SDKService) RemoveDockerContainer(ctx context.Context, id string) error {
	return s.client.Client.ContainerRemove(ctx, id, container.RemoveOptions{Force: true})
}
func (s *SDKService) CreateDockerContainer(ctx context.Context, name, imageName, command string) (string, error) {
	var commandArgs []string
	if command != "" {
		commandArgs = []string{"/bin/sh", "-c", command}
	}
	return s.CreateContainer(ctx, imageName, name, commandArgs)
}

// CreateContainer exposes the command-vector form used by deployment adapters.
func (s *SDKService) CreateContainer(ctx context.Context, imageName, containerName string, command []string) (string, error) {
	config := &container.Config{Image: imageName, Tty: true, Cmd: command}
	host := &container.HostConfig{}
	imageInfo, _, err := s.client.Client.ImageInspectWithRaw(ctx, imageName)
	if err == nil && imageInfo.Config != nil && len(imageInfo.Config.ExposedPorts) > 0 {
		config.ExposedPorts = imageInfo.Config.ExposedPorts
		host.PortBindings = make(nat.PortMap)
		for port := range imageInfo.Config.ExposedPorts {
			host.PortBindings[port] = []nat.PortBinding{{HostIP: "0.0.0.0"}}
		}
	}
	created, err := s.client.Client.ContainerCreate(ctx, config, host, nil, nil, containerName)
	if err != nil {
		return "", err
	}
	return created.ID, nil
}
func (s *SDKService) ListDockerImages(ctx context.Context) ([]image.Summary, error) {
	return s.client.Client.ImageList(ctx, types.ImageListOptions{})
}
func (s *SDKService) PullDockerImage(ctx context.Context, name string, onProgress func(string) error) error {
	imageName := strings.TrimSpace(name)
	if !strings.Contains(imageName, "/") {
		imageName = "library/" + imageName
	}
	reader, err := s.client.Client.ImagePull(ctx, imageName, types.ImagePullOptions{})
	if err != nil {
		return err
	}
	defer reader.Close()
	decoder := json.NewDecoder(reader)
	for {
		var event struct {
			Status   string `json:"status"`
			Error    string `json:"error"`
			Progress string `json:"progress"`
		}
		if err := decoder.Decode(&event); err != nil {
			if err == io.EOF {
				return nil
			}
			return err
		}
		if event.Error != "" {
			return fmt.Errorf("docker image pull: %s", event.Error)
		}
		if event.Status != "" {
			message := event.Status
			if event.Progress != "" {
				message += ": " + event.Progress
			}
			if onProgress != nil {
				if err := onProgress(message); err != nil {
					return err
				}
			}
		}
	}
}
func (s *SDKService) RemoveDockerImage(ctx context.Context, id string) error {
	_, err := s.client.Client.ImageRemove(ctx, id, types.ImageRemoveOptions{Force: true})
	return err
}
func (s *SDKService) ListDockerVolumes(ctx context.Context) (volume.ListResponse, error) {
	return s.client.Client.VolumeList(ctx, volume.ListOptions{})
}
func (s *SDKService) RemoveDockerVolume(ctx context.Context, name string) error {
	return s.client.Client.VolumeRemove(ctx, name, true)
}
func (s *SDKService) CreateDockerVolume(ctx context.Context, name, driver string) error {
	_, err := s.client.Client.VolumeCreate(ctx, volume.CreateOptions{Name: name, Driver: driver})
	return err
}
func (s *SDKService) ListDockerNetworks(ctx context.Context) ([]types.NetworkResource, error) {
	return s.client.Client.NetworkList(ctx, types.NetworkListOptions{})
}
func (s *SDKService) RemoveDockerNetwork(ctx context.Context, id string) error {
	return s.client.Client.NetworkRemove(ctx, id)
}
func (s *SDKService) CreateDockerNetwork(ctx context.Context, name, driver, subnet, gateway, ipRange string) error {
	if driver == "" {
		driver = "bridge"
	}
	opts := types.NetworkCreate{Driver: driver}
	if subnet != "" || gateway != "" || ipRange != "" {
		opts.IPAM = &network.IPAM{Driver: "default", Config: []network.IPAMConfig{{Subnet: subnet, Gateway: gateway, IPRange: ipRange}}}
	}
	_, err := s.client.Client.NetworkCreate(ctx, name, opts)
	return err
}

package docker

import (
	"github.com/docker/docker/client"
)

// DockerClient owns the official SDK connection used by the Docker module.
// Keeping the raw SDK handle available lets applications add narrow adapters
// while reusable Docker operations migrate into this package.
type DockerClient struct {
	Client         *client.Client
	RegistryMirror string
}

// NewDockerClient creates a Docker SDK client from the standard DOCKER_HOST,
// TLS and API-version environment variables.
func NewDockerClient(registryMirror string) (*DockerClient, error) {
	c, err := client.NewClientWithOpts(client.FromEnv)
	if err != nil {
		return nil, err
	}
	return &DockerClient{Client: c, RegistryMirror: registryMirror}, nil
}

func (c *DockerClient) Close() error {
	if c == nil || c.Client == nil {
		return nil
	}
	return c.Client.Close()
}

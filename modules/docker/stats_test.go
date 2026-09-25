package docker

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/docker/docker/client"
)

func TestDockerStatsDecodesDaemonStream(t *testing.T) {
	daemon := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || !strings.HasSuffix(r.URL.Path, "/containers/container-1/stats") {
			t.Errorf("unexpected Docker API request: %s %s", r.Method, r.URL.Path)
			http.NotFound(w, r)
			return
		}
		_, _ = w.Write([]byte(`{"cpu_stats":{"cpu_usage":{"total_usage":200,"percpu_usage":[100,100]},"system_cpu_usage":1000},"precpu_stats":{"cpu_usage":{"total_usage":100,"percpu_usage":[50,50]},"system_cpu_usage":500},"memory_stats":{"usage":4096,"limit":8192},"networks":{"eth0":{"rx_bytes":100,"tx_bytes":200},"eth1":{"rx_bytes":300,"tx_bytes":400}}}`))
	}))
	defer daemon.Close()
	dockerClient, err := client.NewClientWithOpts(client.WithHost(daemon.URL), client.WithVersion("1.41"), client.WithHTTPClient(daemon.Client()))
	if err != nil {
		t.Fatal(err)
	}
	defer dockerClient.Close()
	service := NewLocalService(t.Context(), &DockerClient{Client: dockerClient}, nil, "", "")
	stats, err := service.GetDockerContainerStats(t.Context(), "container-1")
	if err != nil {
		t.Fatalf("get container stats: %v", err)
	}
	if stats.CPUPercent != 40 || stats.MemoryUsage != 4096 || stats.MemoryLimit != 8192 || stats.MemoryPercent != 50 || stats.NetworkRx != 400 || stats.NetworkTx != 600 {
		t.Fatalf("container stats = %+v", stats)
	}
}

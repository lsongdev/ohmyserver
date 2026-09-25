package web

import (
	"context"
	"encoding/json"
	"fmt"
	speedtestmodule "github.com/lsongdev/ohmyserver/modules/speedtest"
	"github.com/lsongdev/speedtest-go/librespeed"
	speedtestlib "github.com/lsongdev/speedtest-go/speedtest"
	"github.com/lsongdev/speedtest-go/types"
	"log"
	"net/http"
	"net/url"
	"time"
)

type LocalService struct{ repo *Repository }

func NewLocalService(repo *Repository) *LocalService { return &LocalService{repo: repo} }
func (s *LocalService) ListResults(ctx context.Context) ([]speedtestmodule.Result, error) {
	return s.repo.List(ctx)
}
func (s *LocalService) DeleteResult(ctx context.Context, id int64) error {
	return s.repo.Delete(ctx, id)
}
func (s *LocalService) ListServers(ctx context.Context, kind string) ([]speedtestmodule.ServerInfo, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if kind == "librespeed" {
		return getLibrespeedServers()
	}
	if kind != "speedtest" {
		return nil, fmt.Errorf("unsupported speedtest type %q", kind)
	}
	return getOoklaServers()
}
func (s *LocalService) RunSpeedtest(ctx context.Context, w http.ResponseWriter, r *http.Request) {
	if ctx.Err() != nil {
		return
	}
	defer r.Body.Close()
	var input speedtestmodule.RunRequest
	d := json.NewDecoder(http.MaxBytesReader(w, r.Body, 16<<10))
	d.DisallowUnknownFields()
	if e := d.Decode(&input); e != nil {
		http.Error(w, "invalid speedtest request", http.StatusBadRequest)
		return
	}
	if (input.Type != "speedtest" && input.Type != "librespeed") || input.ServerID == "" {
		http.Error(w, "type and server_id are required", http.StatusUnprocessableEntity)
		return
	}
	clone := r.Clone(ctx)
	clone.URL = new(url.URL)
	*clone.URL = *r.URL
	q := clone.URL.Query()
	q.Set("type", input.Type)
	q.Set("server", input.ServerID)
	clone.URL.RawQuery = q.Encode()
	s.runSpeedtestStream(w, clone)
}
func getLibrespeedServers() ([]speedtestmodule.ServerInfo, error) {
	servers, err := librespeed.FetchServers()
	if err != nil {
		log.Printf("Error fetching librespeed servers: %v", err)
		return nil, err
	}

	var result []speedtestmodule.ServerInfo
	for _, s := range servers {
		result = append(result, speedtestmodule.ServerInfo{
			ID:   s.Server,
			Name: s.Name,
			Host: s.Server,
		})
	}
	return result, nil
}

func getOoklaServers() ([]speedtestmodule.ServerInfo, error) {
	servers, err := speedtestlib.FetchServers()
	if err != nil {
		log.Printf("Error fetching speedtest servers: %v", err)
		return nil, err
	}

	var result []speedtestmodule.ServerInfo
	for _, s := range servers {
		result = append(result, speedtestmodule.ServerInfo{
			ID:   s.Host,
			Name: s.Name,
			Host: s.Host,
		})
	}
	return result, nil
}

func (s *LocalService) runSpeedtestStream(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	if ctx.Err() != nil {
		return
	}
	testType := r.URL.Query().Get("type")
	serverHost := r.URL.Query().Get("server")

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("X-Accel-Buffering", "no")

	flusher, ok := w.(http.Flusher)
	if !ok {
		return
	}

	sendProgress := func(p *speedtestmodule.Progress) {
		if ctx.Err() != nil {
			return
		}
		data, _ := json.Marshal(p)
		fmt.Fprintf(w, "data: %s\n\n", data)
		flusher.Flush()
	}

	sendProgress(&speedtestmodule.Progress{
		Stage:    "connecting",
		Progress: 0,
		Message:  "Connecting to server...",
	})

	var serverName string

	if testType == "librespeed" {
		servers, _ := librespeed.FetchServers()
		if ctx.Err() != nil {
			return
		}
		for _, ls := range servers {
			if ls.Server == serverHost {
				serverName = ls.Name
				break
			}
		}
		if serverName == "" && len(servers) > 0 {
			serverName = servers[0].Name
		}
	} else {
		servers, err := speedtestlib.FetchServers()
		if ctx.Err() != nil {
			return
		}
		if err != nil {
			sendProgress(&speedtestmodule.Progress{
				Stage:    "error",
				Progress: 0,
				Message:  "Failed to get servers: " + err.Error(),
			})
			return
		}
		for _, st := range servers {
			if st.Host == serverHost {
				serverName = st.Name
				break
			}
		}
		if serverName == "" && len(servers) > 0 {
			serverName = servers[0].Name
		}
	}

	if serverName == "" {
		serverName = serverHost
	}

	sendProgress(&speedtestmodule.Progress{
		Stage:    "connecting",
		Progress: 10,
		Message:  "Connected to " + serverName,
	})

	var result speedtestmodule.Result

	if testType == "librespeed" {
		if ctx.Err() != nil {
			return
		}
		sendProgress(&speedtestmodule.Progress{
			Stage:    "download",
			Progress: 20,
			Message:  "Testing download...",
		})

		servers, _ := librespeed.FetchServers()
		if ctx.Err() != nil {
			return
		}
		var lsServer librespeed.Server
		for _, ls := range servers {
			if ls.Server == serverHost {
				lsServer = ls
				break
			}
		}
		if lsServer.Server == "" && len(servers) > 0 {
			lsServer = servers[0]
		}

		client := &librespeed.Client{
			Server:     lsServer,
			Concurrent: 4,
			Duration:   10 * time.Second,
		}

		downResult, err := client.Download(func(state *types.DownloadState) {
			sendProgress(&speedtestmodule.Progress{
				Stage:    "download",
				Progress: 20 + state.Speed/2,
				Download: state.Speed,
				Message:  "Download: " + formatSpeed(state.Speed),
			})
		})
		if ctx.Err() != nil {
			return
		}
		if err != nil {
			sendProgress(&speedtestmodule.Progress{
				Stage:   "error",
				Message: "Download test failed: " + err.Error(),
			})
			return
		}
		if downResult != nil {
			result.Download = downResult.Speed
		}

		sendProgress(&speedtestmodule.Progress{
			Stage:    "upload",
			Progress: 70,
			Message:  "Testing upload...",
		})

		upResult, err := client.Upload(func(state *types.UploadState) {
			sendProgress(&speedtestmodule.Progress{
				Stage:    "upload",
				Progress: 70 + state.Speed/4,
				Upload:   state.Speed,
				Message:  "Upload: " + formatSpeed(state.Speed),
			})
		})
		if ctx.Err() != nil {
			return
		}
		if err != nil {
			sendProgress(&speedtestmodule.Progress{
				Stage:   "error",
				Message: "Upload test failed: " + err.Error(),
			})
			return
		}
		if upResult != nil {
			result.Upload = upResult.Speed
		}

		result.Ping = 0
		result.Jitter = 0
	} else {
		servers, err := speedtestlib.FetchServers()
		if ctx.Err() != nil {
			return
		}
		if err != nil {
			sendProgress(&speedtestmodule.Progress{
				Stage:    "error",
				Progress: 0,
				Message:  "Failed to get servers: " + err.Error(),
			})
			return
		}

		var stServer *speedtestlib.Server
		for i := range servers {
			if servers[i].Host == serverHost {
				stServer = &servers[i]
				break
			}
		}
		if stServer == nil && len(servers) > 0 {
			stServer = &servers[0]
		}

		client := speedtestlib.NewClient(stServer)

		pingResult, err := client.Ping(func(state *types.PingState) {
			latencyMs := float64(state.Latency.Milliseconds())
			sendProgress(&speedtestmodule.Progress{
				Stage:    "connecting",
				Progress: 15,
				Ping:     latencyMs,
				Message:  fmt.Sprintf("Ping: %.0f ms", latencyMs),
			})
		})
		if ctx.Err() != nil {
			return
		}
		if err != nil {
			sendProgress(&speedtestmodule.Progress{
				Stage:   "error",
				Message: "Ping test failed: " + err.Error(),
			})
			return
		}
		if pingResult != nil {
			result.Ping = pingResult.Ping
			result.Jitter = pingResult.Jitter
		}

		sendProgress(&speedtestmodule.Progress{
			Stage:    "download",
			Progress: 20,
			Message:  "Testing download...",
		})

		downResult, err := client.Download(func(state *types.DownloadState) {
			sendProgress(&speedtestmodule.Progress{
				Stage:    "download",
				Progress: 20 + state.Speed/2,
				Download: state.Speed,
				Message:  "Download: " + formatSpeed(state.Speed),
			})
		})
		if ctx.Err() != nil {
			return
		}
		if err != nil {
			sendProgress(&speedtestmodule.Progress{
				Stage:   "error",
				Message: "Download test failed: " + err.Error(),
			})
			return
		}
		if downResult != nil {
			result.Download = downResult.Speed
		}

		sendProgress(&speedtestmodule.Progress{
			Stage:    "upload",
			Progress: 70,
			Message:  "Testing upload...",
		})

		upResult, err := client.Upload(func(state *types.UploadState) {
			sendProgress(&speedtestmodule.Progress{
				Stage:    "upload",
				Progress: 70 + state.Speed/4,
				Upload:   state.Speed,
				Message:  "Upload: " + formatSpeed(state.Speed),
			})
		})
		if ctx.Err() != nil {
			return
		}
		if err != nil {
			sendProgress(&speedtestmodule.Progress{
				Stage:   "error",
				Message: "Upload test failed: " + err.Error(),
			})
			return
		}
		if upResult != nil {
			result.Upload = upResult.Speed
		}
	}

	result.Type = testType
	result.ServerID = serverHost
	result.ServerName = serverName
	result.ServerHost = serverHost

	if ctx.Err() != nil {
		return
	}
	sendProgress(&speedtestmodule.Progress{
		Stage:    "result",
		Progress: 100,
		Download: result.Download,
		Upload:   result.Upload,
		Ping:     result.Ping,
		Jitter:   result.Jitter,
		Message:  fmt.Sprintf("Download: %.2f Mbps, Upload: %.2f Mbps, Ping: %.0f ms", result.Download, result.Upload, result.Ping),
	})

	if err := s.repo.Create(ctx, &result); err != nil {
		log.Printf("Error saving speedtest result: %v", err)
	}
}

func formatSpeed(mbps float64) string {
	if mbps >= 1000 {
		return fmt.Sprintf("%.1f Gbps", mbps/1000)
	}
	return fmt.Sprintf("%.2f Mbps", mbps)
}

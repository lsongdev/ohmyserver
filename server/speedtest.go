package server

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"strconv"
	"time"

	"github.com/lsongdev/speedtest-go/librespeed"
	"github.com/lsongdev/speedtest-go/speedtest"
	"github.com/lsongdev/speedtest-go/types"
)

type SpeedtestResult struct {
	ID         int64     `json:"id"`
	Type       string    `json:"type"`
	ServerID   string    `json:"server_id"`
	ServerName string    `json:"server_name"`
	ServerHost string    `json:"server_host"`
	Download   float64   `json:"download"`
	Upload     float64   `json:"upload"`
	Ping       float64   `json:"ping"`
	Jitter     float64   `json:"jitter"`
	IP         string    `json:"ip"`
	ISP        string    `json:"isp"`
	Country    string    `json:"country"`
	CreatedAt  time.Time `json:"created_at"`
	UpdatedAt  time.Time `json:"updated_at"`
}

type SpeedtestServer struct {
	ID   string `json:"id"`
	Name string `json:"name"`
	Host string `json:"host"`
}

func (s *Server) initSpeedtestSchema() error {
	query := `
	CREATE TABLE IF NOT EXISTS speedtest_results (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		type TEXT NOT NULL,
		server_id TEXT NOT NULL,
		server_name TEXT NOT NULL,
		server_host TEXT NOT NULL,
		download REAL NOT NULL,
		upload REAL NOT NULL,
		ping REAL NOT NULL,
		jitter REAL NOT NULL DEFAULT 0,
		ip TEXT NOT NULL,
		isp TEXT DEFAULT '',
		country TEXT DEFAULT '',
		created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
		updated_at DATETIME DEFAULT CURRENT_TIMESTAMP
	);
	CREATE INDEX IF NOT EXISTS idx_speedtest_created ON speedtest_results(created_at);
	`
	_, err := s.db.Exec(query)
	return err
}

func (s *Server) CreateSpeedtestResult(result *SpeedtestResult) error {

	now := time.Now()
	query := `
	INSERT INTO speedtest_results (type, server_id, server_name, server_host, download, upload, ping, jitter, ip, isp, country, created_at, updated_at)
	VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`
	_, err := s.db.Exec(query, result.Type, result.ServerID, result.ServerName, result.ServerHost,
		result.Download, result.Upload, result.Ping, result.Jitter, result.IP, result.ISP, result.Country, now, now)
	return err
}

func (s *Server) GetAllSpeedtestResults() ([]SpeedtestResult, error) {

	query := `
	SELECT id, type, server_id, server_name, server_host, download, upload, ping, jitter, ip, isp, country, created_at, updated_at
	FROM speedtest_results
	ORDER BY created_at DESC
	LIMIT 100
	`

	rows, err := s.db.Query(query)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var results []SpeedtestResult
	for rows.Next() {
		var r SpeedtestResult
		var createdAt, updatedAt string
		if err := rows.Scan(&r.ID, &r.Type, &r.ServerID, &r.ServerName, &r.ServerHost,
			&r.Download, &r.Upload, &r.Ping, &r.Jitter, &r.IP, &r.ISP, &r.Country, &createdAt, &updatedAt); err != nil {
			return nil, err
		}
		r.CreatedAt = parseTime(createdAt)
		r.UpdatedAt = parseTime(updatedAt)
		results = append(results, r)
	}

	return results, rows.Err()
}

func (s *Server) GetSpeedtestResultByID(id int64) (*SpeedtestResult, error) {

	query := `
	SELECT id, type, server_id, server_name, server_host, download, upload, ping, jitter, ip, isp, country, created_at, updated_at
	FROM speedtest_results
	WHERE id = ?
	`

	var r SpeedtestResult
	var createdAt, updatedAt string
	err := s.db.QueryRow(query, id).Scan(&r.ID, &r.Type, &r.ServerID, &r.ServerName, &r.ServerHost,
		&r.Download, &r.Upload, &r.Ping, &r.Jitter, &r.IP, &r.ISP, &r.Country, &createdAt, &updatedAt)
	if err != nil {
		return nil, err
	}
	r.CreatedAt = parseTime(createdAt)
	r.UpdatedAt = parseTime(updatedAt)

	return &r, nil
}

func (s *Server) DeleteSpeedtestResult(id int64) error {

	query := `DELETE FROM speedtest_results WHERE id = ?`
	_, err := s.db.Exec(query, id)
	return err
}

func (s *Server) GetSpeedtestServers(testType string) ([]SpeedtestServer, error) {
	if testType == "librespeed" {
		return getLibrespeedServers()
	}
	return getOoklaServers()
}

func getLibrespeedServers() ([]SpeedtestServer, error) {
	servers, err := librespeed.FetchServers()
	if err != nil {
		log.Printf("Error fetching librespeed servers: %v", err)
		return nil, err
	}

	var result []SpeedtestServer
	for _, s := range servers {
		result = append(result, SpeedtestServer{
			ID:   s.Server,
			Name: s.Name,
			Host: s.Server,
		})
	}
	return result, nil
}

func getOoklaServers() ([]SpeedtestServer, error) {
	servers, err := speedtest.FetchServers()
	if err != nil {
		log.Printf("Error fetching speedtest servers: %v", err)
		return nil, err
	}

	var result []SpeedtestServer
	for _, s := range servers {
		result = append(result, SpeedtestServer{
			ID:   s.Host,
			Name: s.Name,
			Host: s.Host,
		})
	}
	return result, nil
}

type SpeedtestProgress struct {
	Stage    string  `json:"stage"`
	Progress float64 `json:"progress"`
	Download float64 `json:"download"`
	Upload   float64 `json:"upload"`
	Ping     float64 `json:"ping"`
	Jitter   float64 `json:"jitter"`
	Message  string  `json:"message"`
}

func (s *Server) SpeedtestListView(w http.ResponseWriter, r *http.Request) {
	results, err := s.GetAllSpeedtestResults()
	if err != nil {
		log.Printf("Error fetching speedtest results: %v", err)
		s.Error(w, err)
		return
	}

	s.Render(w, "speedtest/index", H{
		"results": results,
	})
}

func (s *Server) SpeedtestRecordsHandler(w http.ResponseWriter, r *http.Request) {
	results, err := s.GetAllSpeedtestResults()
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(results)
}

func (s *Server) SpeedtestNewHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodGet {
		librespeedServers, _ := s.GetSpeedtestServers("librespeed")
		ooklaServers, _ := s.GetSpeedtestServers("speedtest")

		s.Render(w, "speedtest/new", H{
			"librespeedServers": librespeedServers,
			"ooklaServers":      ooklaServers,
		})
		return
	}

	testType := r.FormValue("type")
	serverID := r.FormValue("server")

	http.Redirect(w, r, fmt.Sprintf("/speedtest/run?type=%s&server=%s", testType, serverID), http.StatusSeeOther)
}

func (s *Server) SpeedtestRunHandler(w http.ResponseWriter, r *http.Request) {
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

	sendProgress := func(p *SpeedtestProgress) {
		data, _ := json.Marshal(p)
		fmt.Fprintf(w, "data: %s\n\n", data)
		flusher.Flush()
	}

	sendProgress(&SpeedtestProgress{
		Stage:    "connecting",
		Progress: 0,
		Message:  "Connecting to server...",
	})

	var serverName string

	if testType == "librespeed" {
		servers, _ := librespeed.FetchServers()
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
		servers, err := speedtest.FetchServers()
		if err != nil {
			sendProgress(&SpeedtestProgress{
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

	sendProgress(&SpeedtestProgress{
		Stage:    "connecting",
		Progress: 10,
		Message:  "Connected to " + serverName,
	})

	var result SpeedtestResult

	if testType == "librespeed" {
		sendProgress(&SpeedtestProgress{
			Stage:    "download",
			Progress: 20,
			Message:  "Testing download...",
		})

		servers, _ := librespeed.FetchServers()
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
			sendProgress(&SpeedtestProgress{
				Stage:    "download",
				Progress: 20 + state.Speed/2,
				Download: state.Speed,
				Message:  "Download: " + formatSpeed(state.Speed),
			})
		})
		if err != nil {
			sendProgress(&SpeedtestProgress{
				Stage:   "error",
				Message: "Download test failed: " + err.Error(),
			})
			return
		}
		if downResult != nil {
			result.Download = downResult.Speed
		}

		sendProgress(&SpeedtestProgress{
			Stage:    "upload",
			Progress: 70,
			Message:  "Testing upload...",
		})

		upResult, err := client.Upload(func(state *types.UploadState) {
			sendProgress(&SpeedtestProgress{
				Stage:    "upload",
				Progress: 70 + state.Speed/4,
				Upload:   state.Speed,
				Message:  "Upload: " + formatSpeed(state.Speed),
			})
		})
		if err != nil {
			sendProgress(&SpeedtestProgress{
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
		servers, err := speedtest.FetchServers()
		if err != nil {
			sendProgress(&SpeedtestProgress{
				Stage:    "error",
				Progress: 0,
				Message:  "Failed to get servers: " + err.Error(),
			})
			return
		}

		var stServer *speedtest.Server
		for i := range servers {
			if servers[i].Host == serverHost {
				stServer = &servers[i]
				break
			}
		}
		if stServer == nil && len(servers) > 0 {
			stServer = &servers[0]
		}

		client := speedtest.NewClient(stServer)

		pingResult, err := client.Ping(func(state *types.PingState) {
			latencyMs := float64(state.Latency.Milliseconds())
			sendProgress(&SpeedtestProgress{
				Stage:    "connecting",
				Progress: 15,
				Ping:     latencyMs,
				Message:  fmt.Sprintf("Ping: %.0f ms", latencyMs),
			})
		})
		if err != nil {
			sendProgress(&SpeedtestProgress{
				Stage:   "error",
				Message: "Ping test failed: " + err.Error(),
			})
			return
		}
		if pingResult != nil {
			result.Ping = pingResult.Ping
			result.Jitter = pingResult.Jitter
		}

		sendProgress(&SpeedtestProgress{
			Stage:    "download",
			Progress: 20,
			Message:  "Testing download...",
		})

		downResult, err := client.Download(func(state *types.DownloadState) {
			sendProgress(&SpeedtestProgress{
				Stage:    "download",
				Progress: 20 + state.Speed/2,
				Download: state.Speed,
				Message:  "Download: " + formatSpeed(state.Speed),
			})
		})
		if err != nil {
			sendProgress(&SpeedtestProgress{
				Stage:   "error",
				Message: "Download test failed: " + err.Error(),
			})
			return
		}
		if downResult != nil {
			result.Download = downResult.Speed
		}

		sendProgress(&SpeedtestProgress{
			Stage:    "upload",
			Progress: 70,
			Message:  "Testing upload...",
		})

		upResult, err := client.Upload(func(state *types.UploadState) {
			sendProgress(&SpeedtestProgress{
				Stage:    "upload",
				Progress: 70 + state.Speed/4,
				Upload:   state.Speed,
				Message:  "Upload: " + formatSpeed(state.Speed),
			})
		})
		if err != nil {
			sendProgress(&SpeedtestProgress{
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

	sendProgress(&SpeedtestProgress{
		Stage:    "result",
		Progress: 100,
		Download: result.Download,
		Upload:   result.Upload,
		Ping:     result.Ping,
		Jitter:   result.Jitter,
		Message:  fmt.Sprintf("Download: %.2f Mbps, Upload: %.2f Mbps, Ping: %.0f ms", result.Download, result.Upload, result.Ping),
	})

	if err := s.CreateSpeedtestResult(&result); err != nil {
		log.Printf("Error saving speedtest result: %v", err)
	}
}

func (s *Server) SpeedtestDeleteHandler(w http.ResponseWriter, r *http.Request) {
	idStr := r.PathValue("id")
	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		s.Error(w, err)
		return
	}

	if err := s.DeleteSpeedtestResult(id); err != nil {
		s.Error(w, err)
		return
	}

	http.Redirect(w, r, "/speedtest", http.StatusSeeOther)
}

func formatSpeed(mbps float64) string {
	if mbps >= 1000 {
		return fmt.Sprintf("%.1f Gbps", mbps/1000)
	}
	return fmt.Sprintf("%.2f Mbps", mbps)
}

func init() {
	RegisterInitSchemaHook((*Server).initSpeedtestSchema)
}

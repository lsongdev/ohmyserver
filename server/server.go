package server

import (
	"context"
	"database/sql"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"html/template"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"reflect"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/lsongdev/ohmyserver/config"
	"github.com/lsongdev/ohmyserver/cron"
	"github.com/lsongdev/ohmyserver/forward"
	"github.com/lsongdev/ohmyserver/web"
	"github.com/lsongdev/yabber"
)

type H map[string]any

// Template helper functions

type Server struct {
	db                    *sql.DB
	config                *config.Config
	docker                *DockerClient
	deployRuntime         deploymentRuntime
	tcpProxy              *forward.TCPProxy
	webServer             *yabber.Server
	webServerMu           sync.Mutex
	acmeHTTP01Mu          sync.RWMutex
	acmeHTTP01            map[string]string
	acmeWorkers           sync.WaitGroup
	acmeContext           context.Context
	acmeCancel            context.CancelFunc
	dnsRuntime            *DNSRuntime
	ntpRuntime            *NTPRuntime
	dhcpRuntime           *DHCPRuntime
	cronScheduler         *cron.Scheduler
	deployments           map[int64]*DeploymentExecutor // Map of deployment ID to executor
	deploymentsMu         sync.RWMutex
	appInstalls           map[string]*AppInstallTask
	appInstallMu          sync.RWMutex
	appsSyncMu            sync.Mutex
	packageManagerFactory func() (packageManagerRuntime, error)
	startedAt             time.Time
	scheduleUpdate        func() error
}

// AppInstallTask tracks the installation progress of an app
type AppInstallTask struct {
	AppID     string
	Status    string // idle, installing, completed, failed
	Logs      []string
	mu        sync.RWMutex
	clients   map[chan string]struct{}
	clientMu  sync.Mutex
	startedAt time.Time
}

func NewServer(cfg *config.Config) (server *Server, err error) {
	d, err := NewDockerClient(cfg.Docker.RegistryMirror)
	if err != nil {
		return
	}

	if err := os.MkdirAll(filepath.Dir(cfg.Database.Path), 0755); err != nil {
		return nil, err
	}

	db, err := sql.Open("sqlite", cfg.Database.Path)
	if err != nil {
		return nil, err
	}

	acmeContext, acmeCancel := context.WithCancel(context.Background())
	server = &Server{
		db:          db,
		docker:      d,
		config:      cfg,
		deployments: make(map[int64]*DeploymentExecutor),
		appInstalls: make(map[string]*AppInstallTask),
		acmeHTTP01:  make(map[string]string),
		acmeContext: acmeContext,
		acmeCancel:  acmeCancel,
		startedAt:   time.Now(),
	}
	if err := server.initSchema(); err != nil {
		db.Close()
		return nil, err
	}
	if err := server.ensureServicePreferences(); err != nil {
		db.Close()
		return nil, err
	}

	// Run init hooks (post-schema initialization)
	server.runInitHooks()
	return
}

// Close closes the database connection and stops all listeners
func (s *Server) Close() error {
	if s.acmeCancel != nil {
		s.acmeCancel()
	}
	s.acmeWorkers.Wait()
	s.runCloseHooks()
	return s.db.Close()
}

func formatTime(t any) string {
	if t == nil {
		return ""
	}

	// Handle int64 (Unix timestamp)
	switch v := t.(type) {
	case int64:
		return time.Unix(v, 0).Format("2006-01-02 15:04:05")
	case int:
		return time.Unix(int64(v), 0).Format("2006-01-02 15:04:05")
	case string:
		if v == "" {
			return ""
		}
		// Try parsing as Unix timestamp string
		if timestamp, err := strconv.ParseInt(v, 10, 64); err == nil {
			return time.Unix(timestamp, 0).Format("2006-01-02 15:04:05")
		}
		// Try parsing as RFC3339 format
		formats := []string{
			time.RFC3339Nano,
			time.RFC3339,
			"2006-01-02T15:04:05.999999999Z",
			"2006-01-02T15:04:05Z",
		}
		for _, format := range formats {
			if parsed, err := time.Parse(format, v); err == nil {
				return parsed.Format("2006-01-02 15:04:05")
			}
		}
		return v
	}

	return ""
}

// formatSize formats file size
func formatSize(bytes int64) string {
	if bytes == 0 {
		return "0 B"
	}
	const k = 1024
	sizes := []string{"B", "KB", "MB", "GB"}
	i := 0
	size := float64(bytes)
	for size >= k && i < len(sizes)-1 {
		size /= k
		i++
	}
	return fmt.Sprintf("%.1f %s", size, sizes[i])
}

// formatDate formats file modification time
func formatDate(t time.Time) string {
	return t.Format("2006-01-02 15:04:05")
}

// formatUptime formats seconds into human-readable uptime
func formatUptime(seconds any) string {
	var s int64
	switch v := seconds.(type) {
	case int:
		s = int64(v)
	case int64:
		s = v
	case uint64:
		s = int64(v)
	case float64:
		s = int64(v)
	default:
		return "0m"
	}
	if s < 0 {
		s = 0
	}
	days := s / 86400
	hours := (s % 86400) / 3600
	minutes := (s % 3600) / 60
	if days > 0 {
		return fmt.Sprintf("%dd %dh %dm", days, hours, minutes)
	} else if hours > 0 {
		return fmt.Sprintf("%dh %dm", hours, minutes)
	}
	return fmt.Sprintf("%dm", minutes)
}

// formatBytes formats bytes into human-readable format
func formatBytes(bytes uint64) string {
	const unit = 1024
	if bytes < unit {
		return fmt.Sprintf("%d B", bytes)
	}
	div, exp := uint64(unit), 0
	for n := bytes / unit; n >= unit; n /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.1f %cB", float64(bytes)/float64(div), "KMGTPE"[exp])
}

// Render renders an HTML template with the provided data.
func (s *Server) Render(w http.ResponseWriter, templateName string, data H) {
	if data == nil {
		data = H{}
	}

	funcMap := template.FuncMap{

		"split":        strings.Split,
		"formatTime":   formatTime,
		"getFileIcon":  getFileIcon,
		"formatSize":   formatSize,
		"formatDate":   formatDate,
		"formatUptime": formatUptime,
		"formatBytes":  formatBytes,
		"toUint64": func(v interface{}) uint64 {
			switch n := v.(type) {
			case int64:
				return uint64(n)
			case int:
				return uint64(n)
			case uint64:
				return n
			default:
				return 0
			}
		},
		"add": func(a, b int64) int64 {
			return a + b
		},
		"mul": func(a, b int64) int64 {
			return a * b
		},
		"div": func(a, b int64) int64 {
			if b == 0 {
				return 0
			}
			return a / b
		},
		"fromUnix": func(unix int64) time.Time {
			return time.Unix(unix, 0)
		},
		"last": func(slice interface{}) interface{} {
			v := reflect.ValueOf(slice)
			if v.Kind() != reflect.Slice || v.Len() == 0 {
				return nil
			}
			return v.Index(v.Len() - 1).Interface()
		},
		"lowercase": func(s string) string {
			return strings.ToLower(s)
		},
		"truncateID": func(id string) string {
			if len(id) >= 12 {
				return id[:12]
			}
			return id
		},
		"toJSON": func(v interface{}) string {
			b, _ := json.Marshal(v)
			return string(b)
		},
		"deref": func(p *int64) int64 {
			if p == nil {
				return 0
			}
			return *p
		},
		"containsInt64": func(values []int64, target int64) bool {
			for _, value := range values {
				if value == target {
					return true
				}
			}
			return false
		},
		"base64url": func(value string) string {
			return base64.RawURLEncoding.EncodeToString([]byte(value))
		},
	}

	templateFiles := []string{"layout.html", templateName + ".html"}
	if strings.HasPrefix(templateName, "webservers/") {
		templateFiles = append(templateFiles, "webservers/nav.html")
	}
	tmpl, err := template.New("").Funcs(funcMap).ParseFS(web.Files, templateFiles...)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	// Execute "index.html" within the layout and write to response
	err = tmpl.ExecuteTemplate(w, "layout", data)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
}

func (s *Server) Error(w http.ResponseWriter, err error) {
	s.Render(w, "error", H{
		"error": err,
	})
}

func (s *Server) IndexView(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/" {
		http.NotFound(w, r)
		return
	}
	widgets, err := s.GetEnabledWidgets()
	if err != nil {
		log.Printf("Error fetching widgets: %v", err)
	}
	s.Render(w, "index", H{"widgets": widgets})
}

func NewSSE(w http.ResponseWriter) (*sse, error) {
	flusher, ok := w.(http.Flusher)
	if !ok {
		return nil, fmt.Errorf("streaming unsupported")
	}

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("Access-Control-Allow-Origin", "*")

	return &sse{
		w:       w,
		flusher: flusher,
	}, nil
}

// ParseFormAll parses both urlencoded and multipart form data.
// Use this instead of r.ParseForm() when handling POST requests
// that may come from fetch() with FormData (multipart/form-data).
func ParseFormAll(r *http.Request) error {
	contentType := r.Header.Get("Content-Type")
	if strings.HasPrefix(contentType, "multipart/form-data") {
		return r.ParseMultipartForm(32 << 20) // 32MB max
	}
	return r.ParseForm()
}

func (s *sse) Send(event, data string) {
	fmt.Fprintf(s.w, "event: %s\ndata: %s\n\n", event, data)
	s.flusher.Flush()
}

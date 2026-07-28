package server

import (
	"encoding/binary"
	"errors"
	"fmt"
	"log"
	"net"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	ntplib "github.com/lsongdev/ntp-go/ntp"
)

type NTPSettings struct {
	Enabled     bool      `json:"enabled"`
	ListenAddr  string    `json:"listen_addr"`
	Stratum     int       `json:"stratum"`
	ReferenceID string    `json:"reference_id"`
	UpdatedAt   time.Time `json:"updated_at"`
}

type NTPRuntimeStatus struct {
	Running    bool
	StartedAt  time.Time
	LastError  string
	ListenAddr string
	Requests   uint64
}

type NTPRuntime struct {
	opMu       sync.Mutex
	mu         sync.RWMutex
	server     *ntplib.Server
	conn       net.PacketConn
	running    bool
	startedAt  time.Time
	lastError  string
	listenAddr string
}

func (s *Server) initNTPSchema() error {
	_, err := s.db.Exec(`
	CREATE TABLE IF NOT EXISTS ntp_settings (
		id INTEGER PRIMARY KEY CHECK (id = 1),
		enabled INTEGER NOT NULL DEFAULT 0,
		listen_addr TEXT NOT NULL DEFAULT ':123',
		stratum INTEGER NOT NULL DEFAULT 2,
		reference_id TEXT NOT NULL DEFAULT 'LOCL',
		updated_at DATETIME DEFAULT CURRENT_TIMESTAMP
	);
	INSERT OR IGNORE INTO ntp_settings (id) VALUES (1);
	`)
	return err
}

func (s *Server) GetNTPSettings() (*NTPSettings, error) {
	var item NTPSettings
	var enabled int
	var updated string
	err := s.db.QueryRow(`
		SELECT enabled, listen_addr, stratum, reference_id, updated_at
		FROM ntp_settings WHERE id = 1
	`).Scan(&enabled, &item.ListenAddr, &item.Stratum, &item.ReferenceID, &updated)
	if err != nil {
		return nil, err
	}
	item.Enabled = enabled != 0
	item.UpdatedAt = parseTime(updated)
	return &item, nil
}

func (s *Server) UpdateNTPSettings(item NTPSettings) error {
	if err := validateNTPSettings(item); err != nil {
		return err
	}
	_, err := s.db.Exec(`
		UPDATE ntp_settings
		SET enabled = ?, listen_addr = ?, stratum = ?, reference_id = ?, updated_at = ?
		WHERE id = 1
	`, item.Enabled, strings.TrimSpace(item.ListenAddr), item.Stratum,
		strings.ToUpper(strings.TrimSpace(item.ReferenceID)), time.Now())
	return err
}

func validateNTPSettings(item NTPSettings) error {
	if _, err := net.ResolveUDPAddr("udp", strings.TrimSpace(item.ListenAddr)); err != nil {
		return fmt.Errorf("invalid NTP listen address: %w", err)
	}
	if item.Stratum < 1 || item.Stratum > 15 {
		return errors.New("NTP stratum must be between 1 and 15")
	}
	if len(strings.TrimSpace(item.ReferenceID)) != 4 {
		return errors.New("NTP reference ID must contain exactly 4 ASCII characters")
	}
	for _, ch := range strings.TrimSpace(item.ReferenceID) {
		if ch < 0x20 || ch > 0x7e {
			return errors.New("NTP reference ID must contain exactly 4 ASCII characters")
		}
	}
	return nil
}

func referenceID(value string) uint32 {
	return binary.BigEndian.Uint32([]byte(strings.ToUpper(value)))
}

func (r *NTPRuntime) Status() NTPRuntimeStatus {
	r.mu.RLock()
	defer r.mu.RUnlock()
	status := NTPRuntimeStatus{
		Running: r.running, StartedAt: r.startedAt, LastError: r.lastError,
		ListenAddr: r.listenAddr,
	}
	if r.server != nil {
		status.Requests = r.server.RequestCount()
	}
	return status
}

func (r *NTPRuntime) Start(settings NTPSettings) error {
	r.opMu.Lock()
	defer r.opMu.Unlock()
	_ = r.stop()
	if !settings.Enabled {
		return nil
	}
	if err := validateNTPSettings(settings); err != nil {
		r.setError(err)
		return err
	}
	conn, err := net.ListenPacket("udp", settings.ListenAddr)
	if err != nil {
		err = fmt.Errorf("listen NTP on %s: %w", settings.ListenAddr, err)
		r.setError(err)
		return err
	}
	ntpServer := ntplib.NewServer(settings.ListenAddr)
	ntpServer.Stratum = uint8(settings.Stratum)
	ntpServer.ReferenceID = referenceID(settings.ReferenceID)

	r.mu.Lock()
	r.server = ntpServer
	r.conn = conn
	r.running = true
	r.startedAt = time.Now()
	r.lastError = ""
	r.listenAddr = conn.LocalAddr().String()
	r.mu.Unlock()

	go func() {
		serveErr := ntpServer.Serve(conn)
		if serveErr != nil && !errors.Is(serveErr, ntplib.ErrServerClosed) {
			r.setError(serveErr)
		}
	}()
	log.Printf("NTP server started on %s", conn.LocalAddr())
	return nil
}

func (r *NTPRuntime) setError(err error) {
	if err == nil {
		return
	}
	r.mu.Lock()
	r.lastError = err.Error()
	r.mu.Unlock()
	log.Printf("NTP server error: %v", err)
}

func (r *NTPRuntime) Stop() error {
	r.opMu.Lock()
	defer r.opMu.Unlock()
	return r.stop()
}

func (r *NTPRuntime) stop() error {
	r.mu.Lock()
	server := r.server
	conn := r.conn
	r.server = nil
	r.conn = nil
	r.running = false
	r.listenAddr = ""
	r.mu.Unlock()
	if conn != nil {
		_ = conn.Close()
	}
	if server != nil {
		return server.Close()
	}
	return nil
}

func (s *Server) reloadNTP() error {
	settings, err := s.GetNTPSettings()
	if err != nil {
		return err
	}
	if s.ntpRuntime == nil {
		s.ntpRuntime = &NTPRuntime{}
	}
	return s.ntpRuntime.Start(*settings)
}

func (s *Server) NTPView(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	settings, err := s.GetNTPSettings()
	if err != nil {
		s.Error(w, err)
		return
	}
	status := NTPRuntimeStatus{}
	if s.ntpRuntime != nil {
		status = s.ntpRuntime.Status()
	}
	s.Render(w, "ntp/index", H{"settings": settings, "status": status})
}

func (s *Server) NTPSettingsHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodGet {
		settings, err := s.GetNTPSettings()
		if err != nil {
			s.Error(w, err)
			return
		}
		s.Render(w, "ntp/settings", H{"settings": settings})
		return
	}
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	previous, err := s.GetNTPSettings()
	if err != nil {
		s.Error(w, err)
		return
	}
	stratum, err := strconv.Atoi(r.FormValue("stratum"))
	if err != nil {
		s.Error(w, errors.New("invalid NTP stratum"))
		return
	}
	settings := NTPSettings{
		Enabled: r.FormValue("enabled") == "1", ListenAddr: r.FormValue("listen_addr"),
		Stratum: stratum, ReferenceID: r.FormValue("reference_id"),
	}
	if err := s.UpdateNTPSettings(settings); err != nil {
		s.Error(w, err)
		return
	}
	if err := s.setServiceDesired("ntp", settings.Enabled); err != nil {
		_ = s.UpdateNTPSettings(*previous)
		s.Error(w, err)
		return
	}
	if settings.Enabled {
		conflicts, conflictErr := s.configuredServiceConflicts("ntp")
		if conflictErr != nil || len(conflicts) > 0 {
			_ = s.UpdateNTPSettings(*previous)
			_ = s.setServiceDesired("ntp", previous.Enabled)
			if conflictErr != nil {
				s.Error(w, conflictErr)
			} else {
				s.Error(w, fmt.Errorf("listener conflict: %s is also used by %s",
					conflicts[0].Endpoint, strings.Join(conflicts[0].Services, ", ")))
			}
			return
		}
	}
	err = s.reloadNTP()
	s.setServiceError("ntp", err)
	if err != nil {
		if rollbackErr := s.UpdateNTPSettings(*previous); rollbackErr == nil {
			_ = s.setServiceDesired("ntp", previous.Enabled)
			_ = s.reloadNTP()
		}
		s.Error(w, err)
		return
	}
	http.Redirect(w, r, "/ntp", http.StatusSeeOther)
}

func (s *Server) NTPActionHandler(w http.ResponseWriter, r *http.Request) {
	s.serviceAction(w, r, "ntp", "/ntp")
}

func init() {
	RegisterInitSchemaHook((*Server).initNTPSchema)
	RegisterInitHook(func(s *Server) error {
		s.ntpRuntime = &NTPRuntime{}
		if !s.serviceDesiredEnabled("ntp", false) {
			return nil
		}
		err := s.reloadNTP()
		s.setServiceError("ntp", err)
		return err
	})
	RegisterCloseHook(func(s *Server) error {
		if s.ntpRuntime == nil {
			return nil
		}
		return s.ntpRuntime.Stop()
	})
}

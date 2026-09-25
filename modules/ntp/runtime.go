package ntp

import (
	"encoding/binary"
	"errors"
	"fmt"
	"log"
	"net"
	"strings"
	"sync"
	"time"

	ntplib "github.com/lsongdev/ntp-go/ntp"
)

type RuntimeStatus struct {
	Running    bool      `json:"running"`
	StartedAt  time.Time `json:"started_at"`
	LastError  string    `json:"last_error"`
	ListenAddr string    `json:"listen_addr"`
	Requests   uint64    `json:"requests"`
}

type Runtime struct {
	opMu       sync.Mutex
	mu         sync.RWMutex
	server     *ntplib.Server
	conn       net.PacketConn
	running    bool
	startedAt  time.Time
	lastError  string
	listenAddr string
}

func ValidateSettings(item Settings) error {
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

func ReferenceID(value string) uint32 {
	return binary.BigEndian.Uint32([]byte(strings.ToUpper(value)))
}

func (r *Runtime) Status() RuntimeStatus {
	r.mu.RLock()
	defer r.mu.RUnlock()
	status := RuntimeStatus{
		Running: r.running, StartedAt: r.startedAt, LastError: r.lastError,
		ListenAddr: r.listenAddr,
	}
	if r.server != nil {
		status.Requests = r.server.RequestCount()
	}
	return status
}

func (r *Runtime) Start(settings Settings) error {
	r.opMu.Lock()
	defer r.opMu.Unlock()
	_ = r.stop()
	if !settings.Enabled {
		return nil
	}
	if err := ValidateSettings(settings); err != nil {
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
	ntpServer.ReferenceID = ReferenceID(settings.ReferenceID)

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

func (r *Runtime) setError(err error) {
	if err == nil {
		return
	}
	r.mu.Lock()
	r.lastError = err.Error()
	r.mu.Unlock()
	log.Printf("NTP server error: %v", err)
}

func (r *Runtime) Stop() error {
	r.opMu.Lock()
	defer r.opMu.Unlock()
	return r.stop()
}

func (r *Runtime) Close() error { return r.Stop() }

func (r *Runtime) stop() error {
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

package server

import (
	"context"
	"crypto/tls"
	"encoding/base64"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	dnsconfig "github.com/lsongdev/dns-go/config"
	"github.com/lsongdev/dns-go/packet"
	"github.com/lsongdev/dns-go/pipeline"
	dnsserver "github.com/lsongdev/dns-go/server"
)

// DNSRuntime owns the embedded dns-go pipeline and its network listeners.
type DNSRuntime struct {
	opMu           sync.Mutex
	mu             sync.RWMutex
	handler        *pipeline.Handler
	closers        []io.Closer
	listenerCancel context.CancelFunc
	refreshCancel  context.CancelFunc
	running        bool
	startedAt      time.Time
	lastError      string
	udpAddr        string
	tcpAddr        string
	dotAddr        string
	dohAddr        string
}

type DNSRuntimeStatus struct {
	Running   bool
	StartedAt time.Time
	LastError string
	UDPAddr   string
	TCPAddr   string
	DoTAddr   string
	DoHAddr   string
}

func (r *DNSRuntime) Status() DNSRuntimeStatus {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return DNSRuntimeStatus{
		Running:   r.running,
		StartedAt: r.startedAt,
		LastError: r.lastError,
		UDPAddr:   r.udpAddr,
		TCPAddr:   r.tcpAddr,
		DoTAddr:   r.dotAddr,
		DoHAddr:   r.dohAddr,
	}
}

func (r *DNSRuntime) setError(err error) {
	if err == nil || errors.Is(err, net.ErrClosed) || errors.Is(err, http.ErrServerClosed) {
		return
	}
	r.mu.Lock()
	r.lastError = err.Error()
	r.mu.Unlock()
	log.Printf("DNS listener error: %v", err)
}

func (r *DNSRuntime) Stop() error {
	r.opMu.Lock()
	defer r.opMu.Unlock()
	return r.stop()
}

func (r *DNSRuntime) stop() error {
	r.mu.Lock()
	closers := r.closers
	handler := r.handler
	cancel := r.listenerCancel
	r.closers = nil
	r.handler = nil
	r.listenerCancel = nil
	r.running = false
	r.udpAddr = ""
	r.tcpAddr = ""
	r.dotAddr = ""
	r.dohAddr = ""
	r.mu.Unlock()

	if cancel != nil {
		cancel()
	}
	var errs []error
	for _, closer := range closers {
		if err := closer.Close(); err != nil && !errors.Is(err, net.ErrClosed) {
			errs = append(errs, err)
		}
	}
	if handler != nil {
		if err := handler.Close(); err != nil {
			errs = append(errs, err)
		}
	}
	return errors.Join(errs...)
}

func (r *DNSRuntime) Start(cfg *dnsconfig.Config) error {
	r.opMu.Lock()
	defer r.opMu.Unlock()
	if err := r.stop(); err != nil {
		log.Printf("DNS shutdown before reload: %v", err)
	}
	if cfg == nil || len(cfg.Listens) == 0 {
		return nil
	}

	handler, err := pipeline.New(cfg)
	if err != nil {
		r.setError(err)
		return err
	}

	var closers []io.Closer
	var udpAddr, tcpAddr, dotAddr, dohAddr string
	listenerCtx, listenerCancel := context.WithCancel(context.Background())
	for _, spec := range cfg.Listens {
		switch spec.Type {
		case "udp":
			conn, listenErr := net.ListenPacket("udp", spec.Addr)
			if listenErr != nil {
				listenerCancel()
				closeDNSClosers(closers)
				handler.Close()
				err = fmt.Errorf("listen udp %s: %w", spec.Addr, listenErr)
				r.setError(err)
				return err
			}
			closers = append(closers, conn)
			udpAddr = conn.LocalAddr().String()
			go func() {
				r.setError(serveDNSUDP(conn, handler))
			}()
		case "tcp":
			ln, listenErr := net.Listen("tcp", spec.Addr)
			if listenErr != nil {
				listenerCancel()
				closeDNSClosers(closers)
				handler.Close()
				err = fmt.Errorf("listen tcp %s: %w", spec.Addr, listenErr)
				r.setError(err)
				return err
			}
			closers = append(closers, ln)
			tcpAddr = ln.Addr().String()
			go func() {
				r.setError(serveDNSTCP(listenerCtx, ln, handler))
			}()
		case "tls", "dot":
			certificate, certErr := tls.LoadX509KeyPair(spec.CertFile, spec.KeyFile)
			if certErr != nil {
				listenerCancel()
				closeDNSClosers(closers)
				handler.Close()
				err = fmt.Errorf("load DNS-over-TLS certificate: %w", certErr)
				r.setError(err)
				return err
			}
			ln, listenErr := net.Listen("tcp", spec.Addr)
			if listenErr != nil {
				listenerCancel()
				closeDNSClosers(closers)
				handler.Close()
				err = fmt.Errorf("listen DNS-over-TLS %s: %w", spec.Addr, listenErr)
				r.setError(err)
				return err
			}
			tlsListener := tls.NewListener(ln, &tls.Config{
				Certificates: []tls.Certificate{certificate},
				MinVersion:   tls.VersionTLS12,
			})
			closers = append(closers, tlsListener)
			dotAddr = tlsListener.Addr().String()
			go func() {
				r.setError(serveDNSTCP(listenerCtx, tlsListener, handler))
			}()
		case "doh", "http":
			ln, listenErr := net.Listen("tcp", spec.Addr)
			if listenErr != nil {
				listenerCancel()
				closeDNSClosers(closers)
				handler.Close()
				err = fmt.Errorf("listen DNS-over-HTTPS %s: %w", spec.Addr, listenErr)
				r.setError(err)
				return err
			}
			httpServer := &http.Server{
				Handler:           dnsHTTPHandler(handler),
				ReadHeaderTimeout: 10 * time.Second,
				IdleTimeout:       60 * time.Second,
			}
			closers = append(closers, httpServer)
			dohAddr = ln.Addr().String()
			go func() {
				r.setError(httpServer.Serve(ln))
			}()
		default:
			listenerCancel()
			closeDNSClosers(closers)
			handler.Close()
			err = fmt.Errorf("embedded DNS listener type %q is not supported", spec.Type)
			r.setError(err)
			return err
		}
	}

	r.mu.Lock()
	r.handler = handler
	r.closers = closers
	r.listenerCancel = listenerCancel
	r.running = true
	r.startedAt = time.Now()
	r.lastError = ""
	r.udpAddr = udpAddr
	r.tcpAddr = tcpAddr
	r.dotAddr = dotAddr
	r.dohAddr = dohAddr
	r.mu.Unlock()
	log.Printf("DNS server started (udp=%s tcp=%s dot=%s doh=%s)", udpAddr, tcpAddr, dotAddr, dohAddr)
	return nil
}

func (r *DNSRuntime) StartRefreshLoop(s *Server) {
	r.mu.Lock()
	if r.refreshCancel != nil {
		r.refreshCancel()
	}
	ctx, cancel := context.WithCancel(context.Background())
	r.refreshCancel = cancel
	r.mu.Unlock()

	go func() {
		ticker := time.NewTicker(30 * time.Minute)
		defer ticker.Stop()
		for {
			select {
			case <-ticker.C:
				if err := s.refreshDNSAndReload(); err != nil {
					r.setError(fmt.Errorf("refresh DNS subscriptions: %w", err))
				}
			case <-ctx.Done():
				return
			}
		}
	}()
}

func (r *DNSRuntime) Close() error {
	r.mu.Lock()
	if r.refreshCancel != nil {
		r.refreshCancel()
		r.refreshCancel = nil
	}
	r.mu.Unlock()
	return r.Stop()
}

func closeDNSClosers(closers []io.Closer) {
	for _, closer := range closers {
		_ = closer.Close()
	}
}

type dnsPacketWriter struct {
	net.PacketConn
	addr net.Addr
}

func (w *dnsPacketWriter) Write(data []byte) (int, error) {
	return w.WriteTo(data, w.addr)
}

func serveDNSUDP(conn net.PacketConn, handler dnsserver.DNSHandler) error {
	buf := make([]byte, 65535)
	for {
		n, remote, err := conn.ReadFrom(buf)
		if err != nil {
			return err
		}
		data := append([]byte(nil), buf[:n]...)
		go func() {
			req, parseErr := packet.FromBytes(data)
			if parseErr != nil {
				log.Printf("DNS UDP packet from %s: %v", remote, parseErr)
				return
			}
			handler.HandleQuery(&dnsserver.PackConn{
				Writer:     &dnsPacketWriter{PacketConn: conn, addr: remote},
				RemoteAddr: remote.String(),
				Request:    req,
			})
		}()
	}
}

func dnsHTTPHandler(handler dnsserver.DNSHandler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		if req.URL.Path != "/dns-query" {
			http.NotFound(w, req)
			return
		}
		var data []byte
		var err error
		switch req.Method {
		case http.MethodGet:
			data, err = base64.RawURLEncoding.DecodeString(req.URL.Query().Get("dns"))
		case http.MethodPost:
			data, err = io.ReadAll(io.LimitReader(req.Body, 65536))
		default:
			w.Header().Set("Allow", "GET, POST")
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		if err != nil || len(data) == 0 {
			http.Error(w, "invalid DNS message", http.StatusBadRequest)
			return
		}
		packet, err := packet.FromBytes(data)
		if err != nil {
			http.Error(w, "invalid DNS message", http.StatusBadRequest)
			return
		}
		w.Header().Set("Content-Type", "application/dns-message")
		w.Header().Set("Cache-Control", "no-store")
		handler.HandleQuery(&dnsserver.PackConn{
			Writer:     w,
			RemoteAddr: req.RemoteAddr,
			Request:    packet,
		})
	})
}

type dnsTCPWriter struct {
	io.Writer
}

func (w *dnsTCPWriter) Write(data []byte) (int, error) {
	if len(data) > 65535 {
		return 0, fmt.Errorf("DNS TCP response exceeds 65535 bytes")
	}
	frame := make([]byte, len(data)+2)
	binary.BigEndian.PutUint16(frame[:2], uint16(len(data)))
	copy(frame[2:], data)
	if _, err := w.Writer.Write(frame); err != nil {
		return 0, err
	}
	return len(data), nil
}

func serveDNSTCP(ctx context.Context, listener net.Listener, handler dnsserver.DNSHandler) error {
	for {
		conn, err := listener.Accept()
		if err != nil {
			return err
		}
		go handleDNSTCPConn(ctx, conn, handler)
	}
}

func handleDNSTCPConn(ctx context.Context, conn net.Conn, handler dnsserver.DNSHandler) {
	defer conn.Close()
	done := make(chan struct{})
	defer close(done)
	go func() {
		select {
		case <-ctx.Done():
			_ = conn.Close()
		case <-done:
		}
	}()
	for {
		_ = conn.SetReadDeadline(time.Now().Add(30 * time.Second))
		var length [2]byte
		if _, err := io.ReadFull(conn, length[:]); err != nil {
			if !errors.Is(err, io.EOF) {
				log.Printf("DNS TCP read from %s: %v", conn.RemoteAddr(), err)
			}
			return
		}
		size := int(binary.BigEndian.Uint16(length[:]))
		if size == 0 {
			return
		}
		data := make([]byte, size)
		if _, err := io.ReadFull(conn, data); err != nil {
			return
		}
		req, err := packet.FromBytes(data)
		if err != nil {
			log.Printf("DNS TCP packet from %s: %v", conn.RemoteAddr(), err)
			continue
		}
		_ = conn.SetWriteDeadline(time.Now().Add(30 * time.Second))
		handler.HandleQuery(&dnsserver.PackConn{
			Writer:     &dnsTCPWriter{Writer: conn},
			RemoteAddr: conn.RemoteAddr().String(),
			Request:    req,
		})
	}
}

func (s *Server) reloadDNS() error {
	cfg, enabled, err := s.buildDNSConfig(context.Background(), false)
	if err != nil {
		return err
	}
	enabled = enabled && s.serviceDesiredEnabled("dns", enabled)
	if s.dnsRuntime == nil {
		s.dnsRuntime = &DNSRuntime{}
	}
	if !enabled {
		return s.dnsRuntime.Stop()
	}
	return s.dnsRuntime.Start(cfg)
}

func (s *Server) refreshDNSAndReload() error {
	cfg, enabled, err := s.buildDNSConfig(context.Background(), true)
	if err != nil {
		return err
	}
	enabled = enabled && s.serviceDesiredEnabled("dns", enabled)
	if s.dnsRuntime == nil {
		s.dnsRuntime = &DNSRuntime{}
	}
	if !enabled {
		return s.dnsRuntime.Stop()
	}
	return s.dnsRuntime.Start(cfg)
}

func (s *Server) dnsListDirectory() string {
	return filepath.Join(filepath.Dir(s.config.Database.Path), "dns-lists")
}

func removeDNSListFile(path string) error {
	if strings.TrimSpace(path) == "" {
		return nil
	}
	err := os.Remove(path)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	return err
}

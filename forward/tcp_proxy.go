package forward

import (
	"context"
	"fmt"
	"io"
	"net"
	"sync"
	"time"
)

// TCPProxyConfig represents a TCP/UDP proxy configuration
type TCPProxyConfig struct {
	ID          int64
	Name        string
	Protocol    string // "tcp" or "udp"
	ListenPort  int
	ForwardHost string
	ForwardPort int
}

// TCPProxy manages TCP/UDP port forwarding
type TCPProxy struct {
	mu        sync.RWMutex
	listeners map[int]*tcpListener
}

type TCPProxyStatus struct {
	ID          int64
	Name        string
	Protocol    string
	ListenPort  int
	ForwardHost string
	ForwardPort int
}

type tcpListener struct {
	ctx      context.Context
	cancel   context.CancelFunc
	listener net.Listener
	config   *TCPProxyConfig
}

// NewTCPProxy creates a new TCP proxy manager
func NewTCPProxy() *TCPProxy {
	return &TCPProxy{
		listeners: make(map[int]*tcpListener),
	}
}

// StartListener starts a TCP/UDP listener
func (p *TCPProxy) StartListener(config *TCPProxyConfig) error {
	p.mu.Lock()
	defer p.mu.Unlock()

	// Stop existing listener on this port
	if p.listeners[config.ListenPort] != nil {
		p.stopListenerLocked(config.ListenPort)
	}

	ctx, cancel := context.WithCancel(context.Background())
	listener := &tcpListener{
		ctx:    ctx,
		cancel: cancel,
		config: config,
	}

	addr := fmt.Sprintf(":%d", config.ListenPort)
	forwardAddr := fmt.Sprintf("%s:%d", config.ForwardHost, config.ForwardPort)

	if config.Protocol == "udp" {
		return p.startUDPListener(listener, addr, forwardAddr)
	}
	return p.startTCPListener(listener, addr, forwardAddr)
}

func (p *TCPProxy) startTCPListener(listener *tcpListener, addr, forwardAddr string) error {
	ln, err := net.Listen("tcp", addr)
	if err != nil {
		return err
	}
	listener.listener = ln

	fmt.Printf("Starting TCP listener %s on %s -> %s\n", listener.config.Name, addr, forwardAddr)

	go func() {
		<-listener.ctx.Done()
		ln.Close()
		fmt.Printf("Stopped TCP listener %s on %s\n", listener.config.Name, addr)
	}()

	go func() {
		for {
			conn, err := ln.Accept()
			if err != nil {
				select {
				case <-listener.ctx.Done():
					return
				default:
					continue
				}
			}
			go p.handleTCPConnection(conn, forwardAddr, listener.config.Name)
		}
	}()

	p.listeners[listener.config.ListenPort] = listener
	return nil
}

func (p *TCPProxy) handleTCPConnection(clientConn net.Conn, forwardAddr, name string) {
	defer clientConn.Close()

	backendConn, err := net.Dial("tcp", forwardAddr)
	if err != nil {
		fmt.Printf("Failed to connect to backend %s: %v\n", forwardAddr, err)
		return
	}
	defer backendConn.Close()

	done := make(chan struct{}, 2)
	go func() {
		io.Copy(backendConn, clientConn)
		done <- struct{}{}
	}()
	go func() {
		io.Copy(clientConn, backendConn)
		done <- struct{}{}
	}()

	<-done
}

func (p *TCPProxy) startUDPListener(listener *tcpListener, addr, forwardAddr string) error {
	conn, err := net.ListenUDP("udp", &net.UDPAddr{Port: listener.config.ListenPort})
	if err != nil {
		return err
	}

	fmt.Printf("Starting UDP listener %s on %s -> %s\n", listener.config.Name, addr, forwardAddr)

	backendAddr, err := net.ResolveUDPAddr("udp", forwardAddr)
	if err != nil {
		conn.Close()
		return err
	}

	backendConn, err := net.DialUDP("udp", nil, backendAddr)
	if err != nil {
		conn.Close()
		return err
	}

	go func() {
		<-listener.ctx.Done()
		conn.Close()
		backendConn.Close()
		fmt.Printf("Stopped UDP listener %s on %s\n", listener.config.Name, addr)
	}()

	go p.handleUDPConnection(conn, backendConn, listener.config.Name)

	p.listeners[listener.config.ListenPort] = listener
	return nil
}

func (p *TCPProxy) handleUDPConnection(clientConn, backendConn *net.UDPConn, name string) {
	buffer := make([]byte, 65535)
	clientAddrMap := make(map[string]*net.UDPAddr)
	var addrMapMu sync.RWMutex

	for {
		clientConn.SetReadDeadline(time.Now().Add(500 * time.Millisecond))
		n, clientAddr, err := clientConn.ReadFromUDP(buffer)
		if err != nil {
			if ne, ok := err.(net.Error); ok && ne.Timeout() {
				continue
			}
			return
		}

		clientKey := clientAddr.String()
		addrMapMu.Lock()
		clientAddrMap[clientKey] = clientAddr
		addrMapMu.Unlock()

		backendConn.Write(buffer[:n])

		go func(cKey string) {
			backendConn.SetReadDeadline(time.Now().Add(2 * time.Second))
			respBuf := make([]byte, 65535)
			n, _, err := backendConn.ReadFromUDP(respBuf)
			if err != nil {
				return
			}

			addrMapMu.RLock()
			addr, exists := clientAddrMap[cKey]
			addrMapMu.RUnlock()

			if !exists {
				return
			}

			clientConn.WriteToUDP(respBuf[:n], addr)
		}(clientKey)
	}
}

// StopListener stops a listener by port
func (p *TCPProxy) StopListener(port int) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.stopListenerLocked(port)
}

func (p *TCPProxy) stopListenerLocked(port int) {
	if listener := p.listeners[port]; listener != nil {
		listener.cancel()
		if listener.listener != nil {
			listener.listener.Close()
		}
		delete(p.listeners, port)
	}
}

// StopAllListeners stops all listeners
func (p *TCPProxy) StopAllListeners() {
	p.mu.Lock()
	defer p.mu.Unlock()
	for port := range p.listeners {
		p.stopListenerLocked(port)
	}
}

// RunningListeners returns a snapshot of active proxy listeners.
func (p *TCPProxy) RunningListeners() []TCPProxyStatus {
	p.mu.RLock()
	defer p.mu.RUnlock()
	items := make([]TCPProxyStatus, 0, len(p.listeners))
	for _, listener := range p.listeners {
		items = append(items, TCPProxyStatus{
			ID:          listener.config.ID,
			Name:        listener.config.Name,
			Protocol:    listener.config.Protocol,
			ListenPort:  listener.config.ListenPort,
			ForwardHost: listener.config.ForwardHost,
			ForwardPort: listener.config.ForwardPort,
		})
	}
	return items
}

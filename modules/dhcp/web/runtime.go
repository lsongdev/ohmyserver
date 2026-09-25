package web

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"log"
	"net"
	"strings"
	"sync"
	"time"

	"github.com/lsongdev/dhcp-go/dhcp4"
	"github.com/lsongdev/dhcp-go/dhcp4/options"
	dhcpmodule "github.com/lsongdev/ohmyserver/modules/dhcp"
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
	server     *dhcp4.Server
	conn       *net.UDPConn
	cancel     context.CancelFunc
	running    bool
	startedAt  time.Time
	lastError  string
	listenAddr string
}

type PacketHandler struct {
	ctx      context.Context
	repo     *Repository
	settings Settings
	pool     *dhcp4.IPPool
	options  []options.Option
}

// NewPacketHandler builds the DHCP protocol handler used by the UDP runtime.
func NewPacketHandler(ctx context.Context, repo *Repository, settings Settings) (*PacketHandler, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if err := dhcpmodule.ValidateSettings(settings); err != nil {
		return nil, err
	}
	serverIP := net.ParseIP(settings.ServerIP).To4()
	mask := net.IPMask(net.ParseIP(settings.SubnetMask).To4())
	network := &net.IPNet{IP: serverIP.Mask(mask), Mask: mask}
	excluded := []net.IP{serverIP}
	if router := net.ParseIP(settings.Router).To4(); router != nil {
		excluded = append(excluded, router)
	}
	pool, err := dhcp4.NewIPPool(network, net.ParseIP(settings.PoolStart), net.ParseIP(settings.PoolEnd), excluded)
	if err != nil {
		return nil, err
	}
	h := &PacketHandler{ctx: ctx, repo: repo, settings: settings, pool: pool}
	h.options = h.responseOptions()
	static, err := repo.StaticLeases(ctx)
	if err != nil {
		return nil, err
	}
	staticByMAC := make(map[string]string, len(static))
	for _, lease := range static {
		mac := strings.ToLower(lease.MAC)
		if err := pool.Assign(mac, net.ParseIP(lease.IP)); err != nil {
			return nil, fmt.Errorf("restore DHCP static lease %s: %w", lease.IP, err)
		}
		staticByMAC[mac] = lease.IP
	}
	if err := repo.CleanupExpired(ctx); err != nil {
		return nil, err
	}
	leases, err := repo.Leases(ctx)
	if err != nil {
		return nil, err
	}
	for _, lease := range leases {
		mac := strings.ToLower(lease.MAC)
		if staticIP, ok := staticByMAC[mac]; ok {
			if lease.IP != staticIP {
				if err := repo.DeleteLeaseByIP(ctx, lease.IP); err != nil {
					return nil, err
				}
			}
			continue
		}
		if err := pool.Assign(mac, net.ParseIP(lease.IP)); err != nil {
			return nil, fmt.Errorf("restore DHCP lease %s: %w", lease.IP, err)
		}
	}
	return h, nil
}

func (h *PacketHandler) responseOptions() []options.Option {
	result := []options.Option{options.NewSubnetMaskOption(h.settings.SubnetMask), options.NewServerIdentifierOption(h.settings.ServerIP), options.NewLeaseTimeOption(uint32(h.settings.LeaseSeconds)), options.NewRenewalTimeOption(uint32(h.settings.RenewalSeconds)), options.NewRebindingTimeOption(uint32(h.settings.RebindingSeconds))}
	if values := dhcpmodule.SplitAddresses(h.settings.Router); len(values) > 0 {
		result = append(result, options.NewRouterOption(values))
	}
	if values := dhcpmodule.SplitAddresses(h.settings.DNSServers); len(values) > 0 {
		result = append(result, options.NewDomainNameServerOption(values))
	}
	if values := dhcpmodule.SplitAddresses(h.settings.NTPServers); len(values) > 0 {
		result = append(result, options.NewNetworkTimeProtocolServersOption(values))
	}
	if h.settings.DomainName != "" {
		result = append(result, options.NewDomainNameOption(h.settings.DomainName))
	}
	if h.settings.BroadcastAddress != "" {
		result = append(result, options.NewBroadcastAddressOption(h.settings.BroadcastAddress))
	}
	return result
}
func (h *PacketHandler) allocate(mac string) (net.IP, error) {
	static, err := h.repo.StaticLeaseByMAC(h.ctx, mac)
	if err == nil {
		ip := net.ParseIP(static.IP)
		if err := h.pool.Assign(mac, ip); err != nil {
			return nil, err
		}
		return ip, nil
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return nil, err
	}
	return h.pool.Allocate(mac)
}
func (h *PacketHandler) persist(mac, ip, hostname, state string) error {
	return h.repo.UpsertLease(h.ctx, strings.ToLower(mac), ip, hostname, state, h.settings.LeaseSeconds)
}
func (h *PacketHandler) HandleDiscover(request *dhcp4.Message, rw dhcp4.OfferWriter) {
	mac := strings.ToLower(request.GetMacAddress())
	ip, err := h.allocate(mac)
	if err != nil {
		log.Printf("DHCP discover from %s: %v", mac, err)
		return
	}
	if err := h.persist(mac, ip.String(), request.GetHostName(), "offered"); err != nil {
		log.Printf("persist DHCP offer for %s: %v", mac, err)
		return
	}
	rw.SendOffer(ip.String(), h.options...)
}
func (h *PacketHandler) HandleRequest(request dhcp4.IGetRequestedIP, rw dhcp4.AckWriter) {
	mac := strings.ToLower(request.GetMacAddress())
	ip, err := h.allocate(mac)
	if err != nil {
		rw.SendNak(err.Error(), h.options...)
		return
	}
	if requested := request.GetRequestedIP(); requested != "" && requested != ip.String() {
		rw.SendNak("requested address is not available", h.options...)
		return
	}
	if err := h.persist(mac, ip.String(), request.GetHostName(), "active"); err != nil {
		log.Printf("persist DHCP lease for %s: %v", mac, err)
		rw.SendNak("unable to persist lease", h.options...)
		return
	}
	rw.SendAck(ip.String(), h.options...)
}
func (h *PacketHandler) HandleRenew(request dhcp4.IGetClientIP, rw dhcp4.AckWriter) {
	mac := strings.ToLower(request.GetMacAddress())
	ip, err := h.allocate(mac)
	if err != nil || request.GetClientIP() != ip.String() {
		rw.SendNak("lease is no longer available", h.options...)
		return
	}
	if err := h.persist(mac, ip.String(), "", "active"); err != nil {
		rw.SendNak("unable to persist lease", h.options...)
		return
	}
	rw.SendAck(ip.String(), h.options...)
}
func (h *PacketHandler) HandleRelease(request dhcp4.IGetClientIP, _ dhcp4.ResponseWriter) {
	ip := request.GetClientIP()
	h.pool.Release(ip)
	if err := h.repo.DeleteLeaseByIP(h.ctx, ip); err != nil {
		log.Printf("release DHCP lease %s: %v", ip, err)
	}
}
func (h *PacketHandler) HandleDecline(request dhcp4.IGetRequestedIP, _ dhcp4.ResponseWriter) {
	ip := request.GetRequestedIP()
	h.pool.Release(ip)
	if err := h.repo.DeleteLeaseByIP(h.ctx, ip); err != nil {
		log.Printf("decline DHCP lease %s: %v", ip, err)
	}
}

func (r *Runtime) Status() RuntimeStatus {
	r.mu.RLock()
	defer r.mu.RUnlock()
	status := RuntimeStatus{Running: r.running, StartedAt: r.startedAt, LastError: r.lastError, ListenAddr: r.listenAddr}
	if r.server != nil {
		status.Requests = r.server.RequestCount()
	}
	return status
}
func (r *Runtime) Start(ctx context.Context, repo *Repository, settings Settings) error {
	r.opMu.Lock()
	defer r.opMu.Unlock()
	_ = r.stop()
	if !settings.Enabled {
		return nil
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	runCtx, cancel := context.WithCancel(ctx)
	handler, err := NewPacketHandler(runCtx, repo, settings)
	if err != nil {
		cancel()
		r.setError(err)
		return err
	}
	addr, err := net.ResolveUDPAddr("udp4", settings.ListenAddr)
	if err != nil {
		cancel()
		r.setError(err)
		return err
	}
	conn, err := net.ListenUDP("udp4", addr)
	if err != nil {
		cancel()
		err = fmt.Errorf("listen DHCP on %s: %w", settings.ListenAddr, err)
		r.setError(err)
		return err
	}
	server := dhcp4.NewServer(settings.ListenAddr, dhcp4.NewDefaultServerMux(handler))
	r.mu.Lock()
	r.server = server
	r.conn = conn
	r.cancel = cancel
	r.running = true
	r.startedAt = time.Now()
	r.lastError = ""
	r.listenAddr = conn.LocalAddr().String()
	r.mu.Unlock()
	go func() {
		serveErr := server.Serve(conn)
		if serveErr != nil && !errors.Is(serveErr, dhcp4.ErrServerClosed) {
			r.setError(serveErr)
		}
	}()
	log.Printf("DHCP server started on %s", conn.LocalAddr())
	return nil
}
func (r *Runtime) setError(err error) {
	if err == nil {
		return
	}
	r.mu.Lock()
	r.lastError = err.Error()
	r.mu.Unlock()
	log.Printf("DHCP server error: %v", err)
}
func (r *Runtime) Stop() error { r.opMu.Lock(); defer r.opMu.Unlock(); return r.stop() }
func (r *Runtime) stop() error {
	r.mu.Lock()
	server, conn, cancel := r.server, r.conn, r.cancel
	r.server = nil
	r.conn = nil
	r.cancel = nil
	r.running = false
	r.listenAddr = ""
	r.mu.Unlock()
	if cancel != nil {
		cancel()
	}
	if conn != nil {
		_ = conn.Close()
	}
	if server != nil {
		return server.Close()
	}
	return nil
}

package server

import (
	"database/sql"
	"errors"
	"fmt"
	"log"
	"net"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/lsongdev/dhcp-go/dhcp4"
	"github.com/lsongdev/dhcp-go/dhcp4/options"
)

type DHCPSettings struct {
	Enabled          bool      `json:"enabled"`
	ListenAddr       string    `json:"listen_addr"`
	ServerIP         string    `json:"server_ip"`
	SubnetMask       string    `json:"subnet_mask"`
	PoolStart        string    `json:"pool_start"`
	PoolEnd          string    `json:"pool_end"`
	Router           string    `json:"router"`
	DNSServers       string    `json:"dns_servers"`
	NTPServers       string    `json:"ntp_servers"`
	DomainName       string    `json:"domain_name"`
	BroadcastAddress string    `json:"broadcast_address"`
	LeaseSeconds     int       `json:"lease_seconds"`
	RenewalSeconds   int       `json:"renewal_seconds"`
	RebindingSeconds int       `json:"rebinding_seconds"`
	UpdatedAt        time.Time `json:"updated_at"`
}

type DHCPStaticLease struct {
	ID          int64     `json:"id"`
	MAC         string    `json:"mac"`
	IP          string    `json:"ip"`
	Hostname    string    `json:"hostname"`
	Description string    `json:"description"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
}

type DHCPLease struct {
	ID        int64     `json:"id"`
	MAC       string    `json:"mac"`
	IP        string    `json:"ip"`
	Hostname  string    `json:"hostname"`
	State     string    `json:"state"`
	ExpiresAt time.Time `json:"expires_at"`
	LastSeen  time.Time `json:"last_seen"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

type DHCPRuntimeStatus struct {
	Running    bool
	StartedAt  time.Time
	LastError  string
	ListenAddr string
	Requests   uint64
}

type DHCPRuntime struct {
	opMu       sync.Mutex
	mu         sync.RWMutex
	server     *dhcp4.Server
	conn       *net.UDPConn
	running    bool
	startedAt  time.Time
	lastError  string
	listenAddr string
}

func (s *Server) initDHCPSchema() error {
	_, err := s.db.Exec(`
	CREATE TABLE IF NOT EXISTS dhcp_settings (
		id INTEGER PRIMARY KEY CHECK (id = 1),
		enabled INTEGER NOT NULL DEFAULT 0,
		listen_addr TEXT NOT NULL DEFAULT ':67',
		server_ip TEXT NOT NULL DEFAULT '192.168.1.1',
		subnet_mask TEXT NOT NULL DEFAULT '255.255.255.0',
		pool_start TEXT NOT NULL DEFAULT '192.168.1.100',
		pool_end TEXT NOT NULL DEFAULT '192.168.1.200',
		router TEXT NOT NULL DEFAULT '192.168.1.1',
		dns_servers TEXT NOT NULL DEFAULT '1.1.1.1
8.8.8.8',
		ntp_servers TEXT NOT NULL DEFAULT '',
		domain_name TEXT NOT NULL DEFAULT 'lan',
		broadcast_address TEXT NOT NULL DEFAULT '192.168.1.255',
		lease_seconds INTEGER NOT NULL DEFAULT 86400,
		renewal_seconds INTEGER NOT NULL DEFAULT 43200,
		rebinding_seconds INTEGER NOT NULL DEFAULT 75600,
		updated_at DATETIME DEFAULT CURRENT_TIMESTAMP
	);
	INSERT OR IGNORE INTO dhcp_settings (id) VALUES (1);

	CREATE TABLE IF NOT EXISTS dhcp_static_leases (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		mac TEXT NOT NULL UNIQUE COLLATE NOCASE,
		ip TEXT NOT NULL UNIQUE,
		hostname TEXT NOT NULL DEFAULT '',
		description TEXT NOT NULL DEFAULT '',
		created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
		updated_at DATETIME DEFAULT CURRENT_TIMESTAMP
	);

	CREATE TABLE IF NOT EXISTS dhcp_leases (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		mac TEXT NOT NULL UNIQUE COLLATE NOCASE,
		ip TEXT NOT NULL UNIQUE,
		hostname TEXT NOT NULL DEFAULT '',
		state TEXT NOT NULL DEFAULT 'active',
		expires_at DATETIME NOT NULL,
		last_seen DATETIME NOT NULL,
		created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
		updated_at DATETIME DEFAULT CURRENT_TIMESTAMP
	);
	CREATE INDEX IF NOT EXISTS idx_dhcp_leases_expires ON dhcp_leases(expires_at);
	`)
	if err != nil {
		return err
	}
	var legacyTable string
	err = s.db.QueryRow(`
		SELECT name FROM sqlite_master WHERE type = 'table' AND name = 'dhcp_reservations'
	`).Scan(&legacyTable)
	if errors.Is(err, sql.ErrNoRows) {
		return nil
	}
	if err != nil {
		return err
	}
	if _, err := s.db.Exec(`
		INSERT OR IGNORE INTO dhcp_static_leases
			(id, mac, ip, hostname, description, created_at, updated_at)
		SELECT id, mac, ip, hostname, description, created_at, updated_at
		FROM dhcp_reservations;
		DROP TABLE dhcp_reservations;
	`); err != nil {
		return fmt.Errorf("migrate DHCP static leases: %w", err)
	}
	return nil
}

func (s *Server) GetDHCPSettings() (*DHCPSettings, error) {
	var item DHCPSettings
	var enabled int
	var updated string
	err := s.db.QueryRow(`
		SELECT enabled, listen_addr, server_ip, subnet_mask, pool_start, pool_end,
		       router, dns_servers, ntp_servers, domain_name, broadcast_address,
		       lease_seconds, renewal_seconds, rebinding_seconds, updated_at
		FROM dhcp_settings WHERE id = 1
	`).Scan(&enabled, &item.ListenAddr, &item.ServerIP, &item.SubnetMask,
		&item.PoolStart, &item.PoolEnd, &item.Router, &item.DNSServers,
		&item.NTPServers, &item.DomainName, &item.BroadcastAddress,
		&item.LeaseSeconds, &item.RenewalSeconds, &item.RebindingSeconds, &updated)
	if err != nil {
		return nil, err
	}
	item.Enabled = enabled != 0
	item.UpdatedAt = parseTime(updated)
	return &item, nil
}

func (s *Server) UpdateDHCPSettings(item DHCPSettings) error {
	if err := validateDHCPSettings(item); err != nil {
		return err
	}
	_, err := s.db.Exec(`
		UPDATE dhcp_settings
		SET enabled = ?, listen_addr = ?, server_ip = ?, subnet_mask = ?,
		    pool_start = ?, pool_end = ?, router = ?, dns_servers = ?,
		    ntp_servers = ?, domain_name = ?, broadcast_address = ?,
		    lease_seconds = ?, renewal_seconds = ?, rebinding_seconds = ?,
		    updated_at = ?
		WHERE id = 1
	`, item.Enabled, strings.TrimSpace(item.ListenAddr), strings.TrimSpace(item.ServerIP),
		strings.TrimSpace(item.SubnetMask), strings.TrimSpace(item.PoolStart),
		strings.TrimSpace(item.PoolEnd), strings.TrimSpace(item.Router),
		strings.TrimSpace(item.DNSServers), strings.TrimSpace(item.NTPServers),
		strings.TrimSpace(item.DomainName), strings.TrimSpace(item.BroadcastAddress),
		item.LeaseSeconds, item.RenewalSeconds, item.RebindingSeconds, time.Now())
	return err
}

func validateDHCPSettings(item DHCPSettings) error {
	if _, err := net.ResolveUDPAddr("udp4", strings.TrimSpace(item.ListenAddr)); err != nil {
		return fmt.Errorf("invalid DHCP listen address: %w", err)
	}
	serverIP := net.ParseIP(strings.TrimSpace(item.ServerIP)).To4()
	maskIP := net.ParseIP(strings.TrimSpace(item.SubnetMask)).To4()
	start := net.ParseIP(strings.TrimSpace(item.PoolStart)).To4()
	end := net.ParseIP(strings.TrimSpace(item.PoolEnd)).To4()
	if serverIP == nil || maskIP == nil || start == nil || end == nil {
		return errors.New("server, subnet mask, and pool addresses must be valid IPv4 addresses")
	}
	mask := net.IPMask(maskIP)
	ones, bits := mask.Size()
	if bits != 32 || ones == 0 {
		return errors.New("subnet mask must be a contiguous IPv4 mask")
	}
	network := &net.IPNet{IP: serverIP.Mask(mask), Mask: mask}
	if !network.Contains(start) || !network.Contains(end) {
		return errors.New("DHCP pool must be inside the server subnet")
	}
	if compareIPv4(start, end) > 0 {
		return errors.New("DHCP pool start must not be after pool end")
	}
	for _, value := range append(splitAddresses(item.DNSServers), splitAddresses(item.NTPServers)...) {
		if net.ParseIP(value).To4() == nil {
			return fmt.Errorf("invalid advertised IPv4 address %q", value)
		}
	}
	for _, value := range []string{item.Router, item.BroadcastAddress} {
		if strings.TrimSpace(value) != "" && net.ParseIP(strings.TrimSpace(value)).To4() == nil {
			return fmt.Errorf("invalid IPv4 address %q", value)
		}
	}
	if item.LeaseSeconds < 60 {
		return errors.New("lease duration must be at least 60 seconds")
	}
	if item.RenewalSeconds <= 0 || item.RenewalSeconds >= item.LeaseSeconds {
		return errors.New("renewal time must be greater than 0 and less than the lease duration")
	}
	if item.RebindingSeconds <= item.RenewalSeconds || item.RebindingSeconds >= item.LeaseSeconds {
		return errors.New("rebinding time must be between renewal time and lease duration")
	}
	return nil
}

func compareIPv4(left, right net.IP) int {
	for i := 0; i < 4; i++ {
		if left[i] < right[i] {
			return -1
		}
		if left[i] > right[i] {
			return 1
		}
	}
	return 0
}

func splitAddresses(value string) []string {
	return strings.FieldsFunc(value, func(r rune) bool {
		return r == ',' || r == '\n' || r == '\r' || r == ' ' || r == '\t'
	})
}

func (s *Server) GetAllDHCPStaticLeases() ([]DHCPStaticLease, error) {
	rows, err := s.db.Query(`
		SELECT id, mac, ip, hostname, description, created_at, updated_at
		FROM dhcp_static_leases ORDER BY ip
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var items []DHCPStaticLease
	for rows.Next() {
		var item DHCPStaticLease
		var created, updated string
		if err := rows.Scan(&item.ID, &item.MAC, &item.IP, &item.Hostname,
			&item.Description, &created, &updated); err != nil {
			return nil, err
		}
		item.CreatedAt, item.UpdatedAt = parseTime(created), parseTime(updated)
		items = append(items, item)
	}
	return items, rows.Err()
}

func (s *Server) GetDHCPStaticLeaseByID(id int64) (*DHCPStaticLease, error) {
	var item DHCPStaticLease
	var created, updated string
	err := s.db.QueryRow(`
		SELECT id, mac, ip, hostname, description, created_at, updated_at
		FROM dhcp_static_leases WHERE id = ?
	`, id).Scan(&item.ID, &item.MAC, &item.IP, &item.Hostname,
		&item.Description, &created, &updated)
	if err != nil {
		return nil, err
	}
	item.CreatedAt, item.UpdatedAt = parseTime(created), parseTime(updated)
	return &item, nil
}

func (s *Server) getDHCPStaticLeaseByMAC(mac string) (*DHCPStaticLease, error) {
	var item DHCPStaticLease
	var created, updated string
	err := s.db.QueryRow(`
		SELECT id, mac, ip, hostname, description, created_at, updated_at
		FROM dhcp_static_leases WHERE mac = ? COLLATE NOCASE
	`, mac).Scan(&item.ID, &item.MAC, &item.IP, &item.Hostname,
		&item.Description, &created, &updated)
	if err != nil {
		return nil, err
	}
	item.CreatedAt, item.UpdatedAt = parseTime(created), parseTime(updated)
	return &item, nil
}

func normalizeMAC(value string) (string, error) {
	mac, err := net.ParseMAC(strings.TrimSpace(value))
	if err != nil || len(mac) != 6 {
		return "", errors.New("invalid Ethernet MAC address")
	}
	return strings.ToLower(mac.String()), nil
}

func (s *Server) saveDHCPStaticLease(id int64, item DHCPStaticLease) error {
	mac, err := normalizeMAC(item.MAC)
	if err != nil {
		return err
	}
	settings, err := s.GetDHCPSettings()
	if err != nil {
		return err
	}
	if err := validateStaticLeaseIP(item.IP, *settings); err != nil {
		return err
	}
	var leaseMAC string
	err = s.db.QueryRow(`
		SELECT mac FROM dhcp_leases WHERE ip = ? AND expires_at > ?
	`, strings.TrimSpace(item.IP), time.Now()).Scan(&leaseMAC)
	if err == nil && !strings.EqualFold(leaseMAC, mac) {
		return fmt.Errorf("address %s is currently leased to %s", item.IP, leaseMAC)
	}
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return err
	}
	now := time.Now()
	if id == 0 {
		_, err = s.db.Exec(`
			INSERT INTO dhcp_static_leases (mac, ip, hostname, description, created_at, updated_at)
			VALUES (?, ?, ?, ?, ?, ?)
		`, mac, strings.TrimSpace(item.IP), strings.TrimSpace(item.Hostname),
			strings.TrimSpace(item.Description), now, now)
		return err
	}
	_, err = s.db.Exec(`
		UPDATE dhcp_static_leases SET mac = ?, ip = ?, hostname = ?,
		       description = ?, updated_at = ? WHERE id = ?
	`, mac, strings.TrimSpace(item.IP), strings.TrimSpace(item.Hostname),
		strings.TrimSpace(item.Description), now, id)
	return err
}

func validateStaticLeaseIP(value string, settings DHCPSettings) error {
	ip := net.ParseIP(strings.TrimSpace(value)).To4()
	if ip == nil {
		return errors.New("static lease must use a valid IPv4 address")
	}
	start, end := net.ParseIP(settings.PoolStart).To4(), net.ParseIP(settings.PoolEnd).To4()
	if compareIPv4(ip, start) < 0 || compareIPv4(ip, end) > 0 {
		return errors.New("static lease address must be inside the configured DHCP pool")
	}
	if ip.Equal(net.ParseIP(settings.ServerIP)) ||
		(strings.TrimSpace(settings.Router) != "" && ip.Equal(net.ParseIP(settings.Router))) {
		return errors.New("static lease address cannot be the DHCP server or router address")
	}
	return nil
}

func (s *Server) GetAllDHCPLeases() ([]DHCPLease, error) {
	rows, err := s.db.Query(`
		SELECT id, mac, ip, hostname, state, expires_at, last_seen, created_at, updated_at
		FROM dhcp_leases ORDER BY ip
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var items []DHCPLease
	for rows.Next() {
		var item DHCPLease
		var expires, seen, created, updated string
		if err := rows.Scan(&item.ID, &item.MAC, &item.IP, &item.Hostname,
			&item.State, &expires, &seen, &created, &updated); err != nil {
			return nil, err
		}
		item.ExpiresAt, item.LastSeen = parseTime(expires), parseTime(seen)
		item.CreatedAt, item.UpdatedAt = parseTime(created), parseTime(updated)
		items = append(items, item)
	}
	return items, rows.Err()
}

func (s *Server) upsertDHCPLease(mac, ip, hostname, state string, leaseSeconds int) error {
	now := time.Now()
	_, err := s.db.Exec(`
		INSERT INTO dhcp_leases
			(mac, ip, hostname, state, expires_at, last_seen, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(mac) DO UPDATE SET
			ip = excluded.ip, hostname = excluded.hostname, state = excluded.state,
			expires_at = excluded.expires_at, last_seen = excluded.last_seen,
			updated_at = excluded.updated_at
	`, strings.ToLower(mac), ip, hostname, state, now.Add(time.Duration(leaseSeconds)*time.Second),
		now, now, now)
	return err
}

func (s *Server) deleteDHCPLeaseByIP(ip string) error {
	_, err := s.db.Exec(`DELETE FROM dhcp_leases WHERE ip = ?`, ip)
	return err
}

func (s *Server) cleanupExpiredDHCPLeases() error {
	_, err := s.db.Exec(`DELETE FROM dhcp_leases WHERE expires_at <= ?`, time.Now())
	return err
}

type embeddedDHCPHandler struct {
	server   *Server
	settings DHCPSettings
	pool     *dhcp4.IPPool
	options  []options.Option
}

func newEmbeddedDHCPHandler(s *Server, settings DHCPSettings) (*embeddedDHCPHandler, error) {
	if err := validateDHCPSettings(settings); err != nil {
		return nil, err
	}
	serverIP := net.ParseIP(settings.ServerIP).To4()
	mask := net.IPMask(net.ParseIP(settings.SubnetMask).To4())
	network := &net.IPNet{IP: serverIP.Mask(mask), Mask: mask}
	excluded := []net.IP{serverIP}
	if router := net.ParseIP(settings.Router).To4(); router != nil {
		excluded = append(excluded, router)
	}
	pool, err := dhcp4.NewIPPool(network, net.ParseIP(settings.PoolStart),
		net.ParseIP(settings.PoolEnd), excluded)
	if err != nil {
		return nil, err
	}
	handler := &embeddedDHCPHandler{server: s, settings: settings, pool: pool}
	handler.options = handler.responseOptions()

	staticLeases, err := s.GetAllDHCPStaticLeases()
	if err != nil {
		return nil, err
	}
	staticLeaseByMAC := make(map[string]string, len(staticLeases))
	for _, staticLease := range staticLeases {
		mac := strings.ToLower(staticLease.MAC)
		if err := pool.Assign(mac, net.ParseIP(staticLease.IP)); err != nil {
			return nil, fmt.Errorf("restore DHCP static lease %s: %w", staticLease.IP, err)
		}
		staticLeaseByMAC[mac] = staticLease.IP
	}
	if err := s.cleanupExpiredDHCPLeases(); err != nil {
		return nil, err
	}
	leases, err := s.GetAllDHCPLeases()
	if err != nil {
		return nil, err
	}
	for _, lease := range leases {
		mac := strings.ToLower(lease.MAC)
		if staticIP, ok := staticLeaseByMAC[mac]; ok {
			if lease.IP != staticIP {
				if err := s.deleteDHCPLeaseByIP(lease.IP); err != nil {
					return nil, err
				}
			}
			continue
		}
		if err := pool.Assign(mac, net.ParseIP(lease.IP)); err != nil {
			return nil, fmt.Errorf("restore DHCP lease %s: %w", lease.IP, err)
		}
	}
	return handler, nil
}

func (h *embeddedDHCPHandler) responseOptions() []options.Option {
	result := []options.Option{
		options.NewSubnetMaskOption(h.settings.SubnetMask),
		options.NewServerIdentifierOption(h.settings.ServerIP),
		options.NewLeaseTimeOption(uint32(h.settings.LeaseSeconds)),
		options.NewRenewalTimeOption(uint32(h.settings.RenewalSeconds)),
		options.NewRebindingTimeOption(uint32(h.settings.RebindingSeconds)),
	}
	if values := splitAddresses(h.settings.Router); len(values) > 0 {
		result = append(result, options.NewRouterOption(values))
	}
	if values := splitAddresses(h.settings.DNSServers); len(values) > 0 {
		result = append(result, options.NewDomainNameServerOption(values))
	}
	if values := splitAddresses(h.settings.NTPServers); len(values) > 0 {
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

func (h *embeddedDHCPHandler) allocate(mac string) (net.IP, error) {
	staticLease, err := h.server.getDHCPStaticLeaseByMAC(mac)
	if err == nil {
		ip := net.ParseIP(staticLease.IP)
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

func (h *embeddedDHCPHandler) HandleDiscover(request *dhcp4.Message, rw dhcp4.OfferWriter) {
	mac := strings.ToLower(request.GetMacAddress())
	ip, err := h.allocate(mac)
	if err != nil {
		log.Printf("DHCP discover from %s: %v", mac, err)
		return
	}
	if err := h.server.upsertDHCPLease(mac, ip.String(), request.GetHostName(),
		"offered", h.settings.LeaseSeconds); err != nil {
		log.Printf("persist DHCP offer for %s: %v", mac, err)
		return
	}
	rw.SendOffer(ip.String(), h.options...)
}

func (h *embeddedDHCPHandler) HandleRequest(request dhcp4.IGetRequestedIP, rw dhcp4.AckWriter) {
	mac := strings.ToLower(request.GetMacAddress())
	ip, err := h.allocate(mac)
	if err != nil {
		rw.SendNak(err.Error(), h.options...)
		return
	}
	requested := request.GetRequestedIP()
	if requested != "" && requested != ip.String() {
		rw.SendNak("requested address is not available", h.options...)
		return
	}
	hostname := request.GetHostName()
	if err := h.server.upsertDHCPLease(mac, ip.String(), hostname,
		"active", h.settings.LeaseSeconds); err != nil {
		log.Printf("persist DHCP lease for %s: %v", mac, err)
		rw.SendNak("unable to persist lease", h.options...)
		return
	}
	rw.SendAck(ip.String(), h.options...)
}

func (h *embeddedDHCPHandler) HandleRenew(request dhcp4.IGetClientIP, rw dhcp4.AckWriter) {
	mac := strings.ToLower(request.GetMacAddress())
	ip, err := h.allocate(mac)
	if err != nil || request.GetClientIP() != ip.String() {
		rw.SendNak("lease is no longer available", h.options...)
		return
	}
	if err := h.server.upsertDHCPLease(mac, ip.String(), "", "active",
		h.settings.LeaseSeconds); err != nil {
		rw.SendNak("unable to persist lease", h.options...)
		return
	}
	rw.SendAck(ip.String(), h.options...)
}

func (h *embeddedDHCPHandler) HandleRelease(request dhcp4.IGetClientIP, _ dhcp4.ResponseWriter) {
	ip := request.GetClientIP()
	h.pool.Release(ip)
	if err := h.server.deleteDHCPLeaseByIP(ip); err != nil {
		log.Printf("release DHCP lease %s: %v", ip, err)
	}
}

func (h *embeddedDHCPHandler) HandleDecline(request dhcp4.IGetRequestedIP, _ dhcp4.ResponseWriter) {
	ip := request.GetRequestedIP()
	h.pool.Release(ip)
	if err := h.server.deleteDHCPLeaseByIP(ip); err != nil {
		log.Printf("decline DHCP lease %s: %v", ip, err)
	}
}

func (r *DHCPRuntime) Status() DHCPRuntimeStatus {
	r.mu.RLock()
	defer r.mu.RUnlock()
	status := DHCPRuntimeStatus{
		Running: r.running, StartedAt: r.startedAt, LastError: r.lastError,
		ListenAddr: r.listenAddr,
	}
	if r.server != nil {
		status.Requests = r.server.RequestCount()
	}
	return status
}

func (r *DHCPRuntime) Start(s *Server, settings DHCPSettings) error {
	r.opMu.Lock()
	defer r.opMu.Unlock()
	_ = r.stop()
	if !settings.Enabled {
		return nil
	}
	handler, err := newEmbeddedDHCPHandler(s, settings)
	if err != nil {
		r.setError(err)
		return err
	}
	addr, err := net.ResolveUDPAddr("udp4", settings.ListenAddr)
	if err != nil {
		r.setError(err)
		return err
	}
	conn, err := net.ListenUDP("udp4", addr)
	if err != nil {
		err = fmt.Errorf("listen DHCP on %s: %w", settings.ListenAddr, err)
		r.setError(err)
		return err
	}
	dhcpServer := dhcp4.NewServer(settings.ListenAddr, dhcp4.NewDefaultServerMux(handler))
	r.mu.Lock()
	r.server = dhcpServer
	r.conn = conn
	r.running = true
	r.startedAt = time.Now()
	r.lastError = ""
	r.listenAddr = conn.LocalAddr().String()
	r.mu.Unlock()
	go func() {
		serveErr := dhcpServer.Serve(conn)
		if serveErr != nil && !errors.Is(serveErr, dhcp4.ErrServerClosed) {
			r.setError(serveErr)
		}
	}()
	log.Printf("DHCP server started on %s", conn.LocalAddr())
	return nil
}

func (r *DHCPRuntime) setError(err error) {
	if err == nil {
		return
	}
	r.mu.Lock()
	r.lastError = err.Error()
	r.mu.Unlock()
	log.Printf("DHCP server error: %v", err)
}

func (r *DHCPRuntime) Stop() error {
	r.opMu.Lock()
	defer r.opMu.Unlock()
	return r.stop()
}

func (r *DHCPRuntime) stop() error {
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

func (s *Server) reloadDHCP() error {
	settings, err := s.GetDHCPSettings()
	if err != nil {
		return err
	}
	if s.dhcpRuntime == nil {
		s.dhcpRuntime = &DHCPRuntime{}
	}
	return s.dhcpRuntime.Start(s, *settings)
}

func (s *Server) DHCPView(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	settings, err := s.GetDHCPSettings()
	if err != nil {
		s.Error(w, err)
		return
	}
	staticLeases, err := s.GetAllDHCPStaticLeases()
	if err != nil {
		s.Error(w, err)
		return
	}
	leases, err := s.GetAllDHCPLeases()
	if err != nil {
		s.Error(w, err)
		return
	}
	status := DHCPRuntimeStatus{}
	if s.dhcpRuntime != nil {
		status = s.dhcpRuntime.Status()
	}
	s.Render(w, "dhcp/index", H{
		"settings": settings, "status": status,
		"staticLeaseCount": len(staticLeases), "leaseCount": len(leases),
	})
}

func (s *Server) DHCPLeaseListView(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	leases, err := s.GetAllDHCPLeases()
	if err != nil {
		s.Error(w, err)
		return
	}
	s.Render(w, "dhcp/leases", H{"leases": leases})
}

func (s *Server) DHCPStaticLeaseListView(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	items, err := s.GetAllDHCPStaticLeases()
	if err != nil {
		s.Error(w, err)
		return
	}
	s.Render(w, "dhcp/static_leases", H{"staticLeases": items})
}

func parsePositiveInt(r *http.Request, name string) (int, error) {
	value, err := strconv.Atoi(r.FormValue(name))
	if err != nil || value <= 0 {
		return 0, fmt.Errorf("%s must be a positive integer", strings.ReplaceAll(name, "_", " "))
	}
	return value, nil
}

func (s *Server) DHCPSettingsHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodGet {
		settings, err := s.GetDHCPSettings()
		if err != nil {
			s.Error(w, err)
			return
		}
		s.Render(w, "dhcp/settings", H{"settings": settings})
		return
	}
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	previous, err := s.GetDHCPSettings()
	if err != nil {
		s.Error(w, err)
		return
	}
	lease, err := parsePositiveInt(r, "lease_seconds")
	if err != nil {
		s.Error(w, err)
		return
	}
	renewal, err := parsePositiveInt(r, "renewal_seconds")
	if err != nil {
		s.Error(w, err)
		return
	}
	rebinding, err := parsePositiveInt(r, "rebinding_seconds")
	if err != nil {
		s.Error(w, err)
		return
	}
	settings := DHCPSettings{
		Enabled: r.FormValue("enabled") == "1", ListenAddr: r.FormValue("listen_addr"),
		ServerIP: r.FormValue("server_ip"), SubnetMask: r.FormValue("subnet_mask"),
		PoolStart: r.FormValue("pool_start"), PoolEnd: r.FormValue("pool_end"),
		Router: r.FormValue("router"), DNSServers: r.FormValue("dns_servers"),
		NTPServers: r.FormValue("ntp_servers"), DomainName: r.FormValue("domain_name"),
		BroadcastAddress: r.FormValue("broadcast_address"), LeaseSeconds: lease,
		RenewalSeconds: renewal, RebindingSeconds: rebinding,
	}
	if err := s.UpdateDHCPSettings(settings); err != nil {
		s.Error(w, err)
		return
	}
	if err := s.setServiceDesired("dhcp", settings.Enabled); err != nil {
		_ = s.UpdateDHCPSettings(*previous)
		s.Error(w, err)
		return
	}
	if settings.Enabled {
		conflicts, conflictErr := s.configuredServiceConflicts("dhcp")
		if conflictErr != nil || len(conflicts) > 0 {
			_ = s.UpdateDHCPSettings(*previous)
			_ = s.setServiceDesired("dhcp", previous.Enabled)
			if conflictErr != nil {
				s.Error(w, conflictErr)
			} else {
				s.Error(w, fmt.Errorf("listener conflict: %s is also used by %s",
					conflicts[0].Endpoint, strings.Join(conflicts[0].Services, ", ")))
			}
			return
		}
	}
	err = s.reloadDHCP()
	s.setServiceError("dhcp", err)
	if err != nil {
		if rollbackErr := s.UpdateDHCPSettings(*previous); rollbackErr == nil {
			_ = s.setServiceDesired("dhcp", previous.Enabled)
			_ = s.reloadDHCP()
		}
		s.Error(w, err)
		return
	}
	http.Redirect(w, r, "/dhcp", http.StatusSeeOther)
}

func (s *Server) DHCPActionHandler(w http.ResponseWriter, r *http.Request) {
	s.serviceAction(w, r, "dhcp", "/dhcp")
}

func staticLeaseFromForm(r *http.Request) DHCPStaticLease {
	return DHCPStaticLease{
		MAC: r.FormValue("mac"), IP: r.FormValue("ip"),
		Hostname: r.FormValue("hostname"), Description: r.FormValue("description"),
	}
}

func (s *Server) DHCPStaticLeaseNewHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodGet {
		s.Render(w, "dhcp/static_lease_form", H{})
		return
	}
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if err := s.saveDHCPStaticLease(0, staticLeaseFromForm(r)); err != nil {
		s.Error(w, err)
		return
	}
	if s.serviceDesiredEnabled("dhcp", false) {
		if err := s.reloadDHCP(); err != nil {
			s.Error(w, err)
			return
		}
	}
	http.Redirect(w, r, "/dhcp/static-leases", http.StatusSeeOther)
}

func dhcpPathID(r *http.Request) (int64, error) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil || id <= 0 {
		return 0, errors.New("invalid ID")
	}
	return id, nil
}

func (s *Server) DHCPStaticLeaseDetailHandler(w http.ResponseWriter, r *http.Request) {
	id, err := dhcpPathID(r)
	if err != nil {
		s.Error(w, err)
		return
	}
	item, err := s.GetDHCPStaticLeaseByID(id)
	if err != nil {
		s.Error(w, err)
		return
	}
	switch r.Method {
	case http.MethodGet:
		s.Render(w, "dhcp/static_lease_detail", H{"staticLease": item})
	case http.MethodDelete:
		if _, err := s.db.Exec(`DELETE FROM dhcp_static_leases WHERE id = ?`, id); err != nil {
			s.Error(w, err)
			return
		}
		if s.serviceDesiredEnabled("dhcp", false) {
			if err := s.reloadDHCP(); err != nil {
				s.Error(w, err)
				return
			}
		}
		w.WriteHeader(http.StatusNoContent)
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

func (s *Server) DHCPStaticLeaseEditHandler(w http.ResponseWriter, r *http.Request) {
	id, err := dhcpPathID(r)
	if err != nil {
		s.Error(w, err)
		return
	}
	item, err := s.GetDHCPStaticLeaseByID(id)
	if err != nil {
		s.Error(w, err)
		return
	}
	if r.Method == http.MethodGet {
		s.Render(w, "dhcp/static_lease_form", H{"staticLease": item})
		return
	}
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if err := s.saveDHCPStaticLease(id, staticLeaseFromForm(r)); err != nil {
		s.Error(w, err)
		return
	}
	if s.serviceDesiredEnabled("dhcp", false) {
		if err := s.reloadDHCP(); err != nil {
			s.Error(w, err)
			return
		}
	}
	http.Redirect(w, r, fmt.Sprintf("/dhcp/static-leases/%d", id), http.StatusSeeOther)
}

func (s *Server) DHCPLeaseDeleteHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodDelete {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	id, err := dhcpPathID(r)
	if err != nil {
		s.Error(w, err)
		return
	}
	if _, err := s.db.Exec(`DELETE FROM dhcp_leases WHERE id = ?`, id); err != nil {
		s.Error(w, err)
		return
	}
	if s.serviceDesiredEnabled("dhcp", false) {
		if err := s.reloadDHCP(); err != nil {
			s.Error(w, err)
			return
		}
	}
	w.WriteHeader(http.StatusNoContent)
}

func init() {
	RegisterInitSchemaHook((*Server).initDHCPSchema)
	RegisterInitHook(func(s *Server) error {
		s.dhcpRuntime = &DHCPRuntime{}
		if !s.serviceDesiredEnabled("dhcp", false) {
			return nil
		}
		err := s.reloadDHCP()
		s.setServiceError("dhcp", err)
		return err
	})
	RegisterCloseHook(func(s *Server) error {
		if s.dhcpRuntime == nil {
			return nil
		}
		return s.dhcpRuntime.Stop()
	})
}

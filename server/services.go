package server

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"net"
	"net/http"
	"sort"
	"strings"
	"time"
)

type ServicePreference struct {
	ID        string    `json:"id"`
	Enabled   bool      `json:"enabled"`
	LastError string    `json:"last_error"`
	UpdatedAt time.Time `json:"updated_at"`
}

func (s *Server) initServicesSchema() error {
	_, err := s.db.Exec(`
	CREATE TABLE IF NOT EXISTS service_preferences (
		service_id TEXT PRIMARY KEY,
		enabled INTEGER NOT NULL DEFAULT 0,
		last_error TEXT NOT NULL DEFAULT '',
		updated_at DATETIME DEFAULT CURRENT_TIMESTAMP
	);
	`)
	return err
}

func (s *Server) ensureServicePreferences() error {
	dnsEnabled := false
	if settings, err := s.GetDNSSettings(); err == nil {
		dnsEnabled = settings.Enabled
	}
	ntpEnabled := false
	if settings, err := s.GetNTPSettings(); err == nil {
		ntpEnabled = settings.Enabled
	}
	dhcpEnabled := false
	if settings, err := s.GetDHCPSettings(); err == nil {
		dhcpEnabled = settings.Enabled
	}
	defaults := []struct {
		id      string
		enabled bool
	}{
		{id: "dns", enabled: dnsEnabled},
		{id: "ntp", enabled: ntpEnabled},
		{id: "dhcp", enabled: dhcpEnabled},
		{id: "web", enabled: true},
		{id: "forward", enabled: true},
	}
	for _, item := range defaults {
		if _, err := s.db.Exec(`
			INSERT OR IGNORE INTO service_preferences (service_id, enabled, updated_at)
			VALUES (?, ?, ?)
		`, item.id, item.enabled, time.Now()); err != nil {
			return err
		}
	}
	return nil
}

func (s *Server) getServicePreference(id string, fallback bool) (ServicePreference, error) {
	var item ServicePreference
	var enabled int
	var updated string
	err := s.db.QueryRow(`
		SELECT service_id, enabled, last_error, updated_at
		FROM service_preferences WHERE service_id = ?
	`, id).Scan(&item.ID, &enabled, &item.LastError, &updated)
	if errors.Is(err, sql.ErrNoRows) {
		return ServicePreference{ID: id, Enabled: fallback}, nil
	}
	if err != nil {
		return ServicePreference{}, err
	}
	item.Enabled = enabled != 0
	item.UpdatedAt = parseTime(updated)
	return item, nil
}

func (s *Server) serviceDesiredEnabled(id string, fallback bool) bool {
	item, err := s.getServicePreference(id, fallback)
	if err != nil {
		return fallback
	}
	return item.Enabled
}

func (s *Server) setServiceDesired(id string, enabled bool) error {
	_, err := s.db.Exec(`
		INSERT INTO service_preferences (service_id, enabled, last_error, updated_at)
		VALUES (?, ?, '', ?)
		ON CONFLICT(service_id) DO UPDATE SET
			enabled = excluded.enabled,
			updated_at = excluded.updated_at
	`, id, enabled, time.Now())
	return err
}

func (s *Server) setServiceError(id string, serviceErr error) {
	message := ""
	if serviceErr != nil {
		message = serviceErr.Error()
	}
	_, _ = s.db.Exec(`
		UPDATE service_preferences SET last_error = ?, updated_at = ? WHERE service_id = ?
	`, message, time.Now(), id)
}

type ServiceConflict struct {
	Endpoint string   `json:"endpoint"`
	Services []string `json:"services"`
}

func (s *Server) serviceAction(w http.ResponseWriter, r *http.Request, serviceID, redirectTo string) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	action := strings.ToLower(r.FormValue("action"))
	ctx, cancel := context.WithTimeout(r.Context(), 20*time.Second)
	defer cancel()

	var err error
	switch action {
	case "start":
		if err = s.validateServiceStart(serviceID); err == nil {
			err = s.setServiceDesired(serviceID, true)
		}
		if err == nil {
			err = s.startService(ctx, serviceID)
		}
	case "stop":
		if err = s.setServiceDesired(serviceID, false); err == nil {
			err = s.stopService(ctx, serviceID)
		}
	case "reload":
		if err = s.validateServiceStart(serviceID); err == nil {
			err = s.reloadService(ctx, serviceID)
		}
	default:
		err = fmt.Errorf("unknown service action %q", action)
	}
	s.setServiceError(serviceID, err)
	if err != nil {
		s.Error(w, err)
		return
	}
	http.Redirect(w, r, redirectTo, http.StatusSeeOther)
}

func (s *Server) startService(_ context.Context, serviceID string) error {
	switch serviceID {
	case "dns":
		settings, err := s.GetDNSSettings()
		if err != nil {
			return err
		}
		settings.Enabled = true
		if err := s.UpdateDNSSettings(*settings); err != nil {
			return err
		}
		return s.refreshDNSAndReload()
	case "ntp":
		settings, err := s.GetNTPSettings()
		if err != nil {
			return err
		}
		settings.Enabled = true
		if err := s.UpdateNTPSettings(*settings); err != nil {
			return err
		}
		return s.reloadNTP()
	case "dhcp":
		settings, err := s.GetDHCPSettings()
		if err != nil {
			return err
		}
		settings.Enabled = true
		if err := s.UpdateDHCPSettings(*settings); err != nil {
			return err
		}
		return s.reloadDHCP()
	case "web":
		return s.applyYabber()
	case "forward":
		return s.startAllProxies()
	}
	return fmt.Errorf("unknown service %q", serviceID)
}

func (s *Server) stopService(_ context.Context, serviceID string) error {
	switch serviceID {
	case "dns":
		settings, err := s.GetDNSSettings()
		if err != nil {
			return err
		}
		settings.Enabled = false
		if err := s.UpdateDNSSettings(*settings); err != nil {
			return err
		}
		if s.dnsRuntime == nil {
			return nil
		}
		return s.dnsRuntime.Stop()
	case "ntp":
		settings, err := s.GetNTPSettings()
		if err != nil {
			return err
		}
		settings.Enabled = false
		if err := s.UpdateNTPSettings(*settings); err != nil {
			return err
		}
		if s.ntpRuntime == nil {
			return nil
		}
		return s.ntpRuntime.Stop()
	case "dhcp":
		settings, err := s.GetDHCPSettings()
		if err != nil {
			return err
		}
		settings.Enabled = false
		if err := s.UpdateDHCPSettings(*settings); err != nil {
			return err
		}
		if s.dhcpRuntime == nil {
			return nil
		}
		return s.dhcpRuntime.Stop()
	case "web":
		return s.stopWebServer()
	case "forward":
		if s.tcpProxy != nil {
			s.tcpProxy.StopAllListeners()
		}
		return nil
	}
	return fmt.Errorf("unknown service %q", serviceID)
}

func (s *Server) reloadService(_ context.Context, serviceID string) error {
	switch serviceID {
	case "dns":
		return s.refreshDNSAndReload()
	case "ntp":
		return s.reloadNTP()
	case "dhcp":
		return s.reloadDHCP()
	case "web":
		return s.applyYabber()
	case "forward":
		if s.tcpProxy != nil {
			s.tcpProxy.StopAllListeners()
		}
		return s.startAllProxies()
	}
	return fmt.Errorf("unknown service %q", serviceID)
}

func (s *Server) validateServiceStart(id string) error {
	conflicts, err := s.configuredServiceConflicts(id)
	if err != nil {
		return err
	}
	if len(conflicts) == 0 {
		return nil
	}
	return fmt.Errorf("listener conflict: %s is also used by %s",
		conflicts[0].Endpoint, strings.Join(conflicts[0].Services, ", "))
}

type configuredEndpoint struct {
	service string
	network string
	address string
}

func (s *Server) configuredServiceConflicts(includeDisabled string) ([]ServiceConflict, error) {
	var endpoints []configuredEndpoint
	add := func(service, network, address string) {
		if strings.TrimSpace(address) != "" {
			endpoints = append(endpoints, configuredEndpoint{service: service, network: network, address: address})
		}
	}
	enabled := func(id string, fallback bool) bool {
		return id == includeDisabled || s.serviceDesiredEnabled(id, fallback)
	}
	if enabled("dns", false) {
		settings, err := s.GetDNSSettings()
		if err != nil {
			return nil, err
		}
		add("DNS Server", "udp", settings.UDPAddr)
		add("DNS Server", "tcp", settings.TCPAddr)
		add("DNS Server", "tcp", settings.DoTAddr)
		add("DNS Server", "tcp", settings.DoHAddr)
	}
	if enabled("ntp", false) {
		settings, err := s.GetNTPSettings()
		if err != nil {
			return nil, err
		}
		add("NTP Server", "udp", settings.ListenAddr)
	}
	if enabled("dhcp", false) {
		settings, err := s.GetDHCPSettings()
		if err != nil {
			return nil, err
		}
		add("DHCP Server", "udp", settings.ListenAddr)
	}
	if enabled("web", true) {
		listeners, err := s.GetAllWebListeners()
		if err != nil {
			return nil, err
		}
		for _, listener := range listeners {
			if listener.Enabled {
				add("Web Server", "tcp", listener.Address)
			}
		}
	}
	if enabled("forward", true) {
		listeners, err := s.GetAllTCPListeners()
		if err != nil {
			return nil, err
		}
		for _, listener := range listeners {
			if listener.Status == "active" {
				add("Port Forwarding", listener.Protocol, fmt.Sprintf(":%d", listener.ListenPort))
			}
		}
	}
	add("OhMyServer", "tcp", s.config.Listen)

	groups := make(map[string][]configuredEndpoint)
	for _, endpoint := range endpoints {
		port := endpointPort(endpoint.address)
		if port == "" {
			continue
		}
		key := strings.ToLower(endpoint.network) + ":" + port
		groups[key] = append(groups[key], endpoint)
	}
	var conflicts []ServiceConflict
	for key, group := range groups {
		services := uniqueServiceNames(group)
		if len(group) < 2 || !endpointGroupConflicts(group) {
			continue
		}
		if includeDisabled != "" && !conflictContainsService(group, includeDisabled) {
			continue
		}
		conflicts = append(conflicts, ServiceConflict{Endpoint: key, Services: services})
	}
	sort.Slice(conflicts, func(i, j int) bool { return conflicts[i].Endpoint < conflicts[j].Endpoint })
	return conflicts, nil
}

func endpointPort(address string) string {
	_, port, err := net.SplitHostPort(strings.TrimSpace(address))
	if err == nil {
		return port
	}
	if strings.HasPrefix(address, ":") {
		return strings.TrimPrefix(address, ":")
	}
	return ""
}

func endpointHost(address string) string {
	host, _, err := net.SplitHostPort(strings.TrimSpace(address))
	if err != nil {
		return ""
	}
	return strings.Trim(host, "[]")
}

func endpointGroupConflicts(group []configuredEndpoint) bool {
	for i := 0; i < len(group); i++ {
		for j := i + 1; j < len(group); j++ {
			left := endpointHost(group[i].address)
			right := endpointHost(group[j].address)
			if wildcardHost(left) || wildcardHost(right) || strings.EqualFold(left, right) {
				return true
			}
		}
	}
	return false
}

func wildcardHost(host string) bool {
	return host == "" || host == "0.0.0.0" || host == "::"
}

func uniqueServiceNames(group []configuredEndpoint) []string {
	seen := make(map[string]bool)
	var names []string
	for _, endpoint := range group {
		if !seen[endpoint.service] {
			seen[endpoint.service] = true
			names = append(names, endpoint.service)
		}
	}
	sort.Strings(names)
	return names
}

func conflictContainsService(group []configuredEndpoint, id string) bool {
	name := map[string]string{
		"dns": "DNS Server", "ntp": "NTP Server", "dhcp": "DHCP Server",
		"web": "Web Server", "forward": "Port Forwarding",
	}[id]
	for _, endpoint := range group {
		if endpoint.service == name {
			return true
		}
	}
	return false
}

func (s *Server) LegacyDomainsRedirect(w http.ResponseWriter, r *http.Request) {
	status := http.StatusPermanentRedirect
	if r.Method != http.MethodGet && r.Method != http.MethodHead {
		status = http.StatusSeeOther
	}
	http.Redirect(w, r, "/dns", status)
}

// LegacyListenersRedirect preserves old bookmarks while making /forward the
// canonical location for Layer 4 forwarding configuration.
func (s *Server) LegacyListenersRedirect(w http.ResponseWriter, r *http.Request) {
	path := r.URL.Path
	if strings.HasPrefix(path, "/listeners") {
		path = "/forward" + strings.TrimPrefix(path, "/listeners")
	} else if strings.HasPrefix(path, "/proxy") {
		path = "/forward" + strings.TrimPrefix(path, "/proxy")
	}
	target := path
	if r.URL.RawQuery != "" {
		target += "?" + r.URL.RawQuery
	}
	http.Redirect(w, r, target, http.StatusPermanentRedirect)
}

func init() {
	RegisterInitSchemaHook((*Server).initServicesSchema)
}

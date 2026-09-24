package host

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"net"
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

func parseServiceTime(value string) time.Time {
	for _, layout := range []string{time.RFC3339Nano, "2006-01-02 15:04:05.999999999-07:00", "2006-01-02 15:04:05.999999999", "2006-01-02 15:04:05"} {
		if parsed, err := time.Parse(layout, value); err == nil {
			return parsed
		}
	}
	return time.Time{}
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
	if settings, err := s.DNSRepository().GetSettings(context.Background()); err == nil {
		dnsEnabled = settings.Enabled
	}
	ntpEnabled := false
	if settings, err := s.NTPRepository().Get(context.Background()); err == nil {
		ntpEnabled = settings.Enabled
	}
	dhcpEnabled := false
	if settings, err := s.dhcpRepository().Settings(context.Background()); err == nil {
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
	return s.getServicePreferenceContext(context.Background(), id, fallback)
}

func (s *Server) getServicePreferenceContext(ctx context.Context, id string, fallback bool) (ServicePreference, error) {
	if err := ctx.Err(); err != nil {
		return ServicePreference{}, err
	}
	var item ServicePreference
	var enabled int
	var updated string
	err := s.db.QueryRowContext(ctx, `
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
	item.UpdatedAt = parseServiceTime(updated)
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
	return s.setServiceDesiredContext(context.Background(), id, enabled)
}

func (s *Server) setServiceDesiredContext(ctx context.Context, id string, enabled bool) error {
	_, err := s.db.ExecContext(ctx, `
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

func (s *Server) startService(ctx context.Context, serviceID string) error {
	switch serviceID {
	case "dns":
		return s.dnsLocalService(ctx).DNSAction(ctx, "start")
	case "ntp":
		return s.ntpModuleService().NTPAction(ctx, "start")
	case "dhcp":
		return s.dhcpModuleService().DHCPAction(ctx, "start")
	case "web":
		return s.applyYabber(ctx)
	case "forward":
		return s.reloadForward(ctx)
	}
	return fmt.Errorf("unknown service %q", serviceID)
}

func (s *Server) stopService(ctx context.Context, serviceID string) error {
	switch serviceID {
	case "dns":
		return s.dnsLocalService(ctx).DNSAction(ctx, "stop")
	case "ntp":
		return s.ntpModuleService().NTPAction(ctx, "stop")
	case "dhcp":
		return s.dhcpModuleService().DHCPAction(ctx, "stop")
	case "web":
		return s.stopWebServer()
	case "forward":
		s.forwardLocalService().Close()
		return nil
	}
	return fmt.Errorf("unknown service %q", serviceID)
}

func (s *Server) reloadService(ctx context.Context, serviceID string) error {
	switch serviceID {
	case "dns":
		return s.dnsLocalService(ctx).DNSAction(ctx, "reload")
	case "ntp":
		return s.ntpModuleService().NTPAction(ctx, "reload")
	case "dhcp":
		return s.dhcpModuleService().DHCPAction(ctx, "reload")
	case "web":
		return s.applyYabber(ctx)
	case "forward":
		return s.reloadForward(ctx)
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
		settings, err := s.DNSRepository().GetSettings(context.Background())
		if err != nil {
			return nil, err
		}
		add("DNS Server", "udp", settings.UDPAddr)
		add("DNS Server", "tcp", settings.TCPAddr)
		add("DNS Server", "tcp", settings.DoTAddr)
		add("DNS Server", "tcp", settings.DoHAddr)
	}
	if enabled("ntp", false) {
		settings, err := s.NTPRepository().Get(context.Background())
		if err != nil {
			return nil, err
		}
		add("NTP Server", "udp", settings.ListenAddr)
	}
	if enabled("dhcp", false) {
		settings, err := s.dhcpRepository().Settings(context.Background())
		if err != nil {
			return nil, err
		}
		add("DHCP Server", "udp", settings.ListenAddr)
	}
	if enabled("web", true) {
		listeners, err := s.WebServerRepository().ListListeners(context.Background())
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
		listeners, err := s.ForwardRepository().List(context.Background())
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

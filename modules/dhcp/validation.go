package dhcp

import (
	"errors"
	"fmt"
	"net"
	"strings"
)

func ValidateSettings(item Settings) error {
	if _, err := net.ResolveUDPAddr("udp4", strings.TrimSpace(item.ListenAddr)); err != nil {
		return fmt.Errorf("invalid DHCP listen address: %w", err)
	}
	serverIP, maskIP := net.ParseIP(strings.TrimSpace(item.ServerIP)).To4(), net.ParseIP(strings.TrimSpace(item.SubnetMask)).To4()
	start, end := net.ParseIP(strings.TrimSpace(item.PoolStart)).To4(), net.ParseIP(strings.TrimSpace(item.PoolEnd)).To4()
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
	if CompareIPv4(start, end) > 0 {
		return errors.New("DHCP pool start must not be after pool end")
	}
	for _, value := range append(SplitAddresses(item.DNSServers), SplitAddresses(item.NTPServers)...) {
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

// NormalizeSettings trims user supplied strings and validates the resulting
// configuration before it is persisted or applied to the DHCP runtime.
func NormalizeSettings(item Settings) (Settings, error) {
	item.ListenAddr = strings.TrimSpace(item.ListenAddr)
	item.ServerIP = strings.TrimSpace(item.ServerIP)
	item.SubnetMask = strings.TrimSpace(item.SubnetMask)
	item.PoolStart = strings.TrimSpace(item.PoolStart)
	item.PoolEnd = strings.TrimSpace(item.PoolEnd)
	item.Router = strings.TrimSpace(item.Router)
	item.DNSServers = strings.TrimSpace(item.DNSServers)
	item.NTPServers = strings.TrimSpace(item.NTPServers)
	item.DomainName = strings.TrimSpace(item.DomainName)
	item.BroadcastAddress = strings.TrimSpace(item.BroadcastAddress)
	if err := ValidateSettings(item); err != nil {
		return Settings{}, err
	}
	return item, nil
}

func ValidateStaticLeaseIP(value string, settings Settings) error {
	ip := net.ParseIP(strings.TrimSpace(value)).To4()
	if ip == nil {
		return errors.New("static lease must use a valid IPv4 address")
	}
	start, end := net.ParseIP(settings.PoolStart).To4(), net.ParseIP(settings.PoolEnd).To4()
	if CompareIPv4(ip, start) < 0 || CompareIPv4(ip, end) > 0 {
		return errors.New("static lease address must be inside the configured DHCP pool")
	}
	if ip.Equal(net.ParseIP(settings.ServerIP)) || (strings.TrimSpace(settings.Router) != "" && ip.Equal(net.ParseIP(settings.Router))) {
		return errors.New("static lease address cannot be the DHCP server or router address")
	}
	return nil
}
func NormalizeMAC(value string) (string, error) {
	mac, err := net.ParseMAC(strings.TrimSpace(value))
	if err != nil || len(mac) != 6 {
		return "", errors.New("invalid Ethernet MAC address")
	}
	return strings.ToLower(mac.String()), nil
}
func CompareIPv4(left, right net.IP) int {
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
func SplitAddresses(value string) []string {
	return strings.FieldsFunc(value, func(r rune) bool { return r == ',' || r == '\n' || r == '\r' || r == ' ' || r == '\t' })
}

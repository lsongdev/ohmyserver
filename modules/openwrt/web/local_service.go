package web

import (
	"context"
	"fmt"
	"net"
	"net/url"
	"time"

	"github.com/lsongdev/openwrt-go/openwrt"
)

type LocalService struct{ repo *Repository }

func NewLocalService(repo *Repository) *LocalService { return &LocalService{repo: repo} }

var _ Runtime = (*LocalService)(nil)

func (s *LocalService) openWrtClient(ctx context.Context, id int64) (*openwrt.Client, error) {
	stored, err := s.repo.Get(ctx, id)
	if err != nil {
		return nil, err
	}
	client := openwrt.NewClient((&url.URL{Scheme: stored.Protocol, Host: net.JoinHostPort(stored.Host, fmt.Sprint(stored.Port))}).String(),
		openwrt.WithCredentials(stored.Username, stored.Password), openwrt.WithTimeout(30*time.Second))
	if err := client.Login(ctx); err != nil {
		return nil, fmt.Errorf("connect to OpenWrt router: %w", err)
	}
	return client, nil
}

func (s *LocalService) withOpenWrt(ctx context.Context, id int64, fn func(*openwrt.Client) (any, error)) (any, error) {
	client, err := s.openWrtClient(ctx, id)
	if err != nil {
		return nil, err
	}
	defer client.Logout(ctx)
	return fn(client)
}

func (s *LocalService) ReadOpenWrt(ctx context.Context, id int64, section string) (any, error) {
	return s.withOpenWrt(ctx, id, func(client *openwrt.Client) (any, error) {
		network := openwrt.NewNetworkService(client)
		firewall := openwrt.NewFirewallService(client)
		switch section {
		case "overview":
			info, err := client.GetSystemInfo(ctx)
			if err != nil {
				return nil, err
			}
			return map[string]any{"system": info, "services": listOpenWrtServices(ctx, client)}, nil
		case "network":
			interfaces, err := network.ListInterfaces(ctx)
			if err != nil {
				return nil, err
			}
			wifi, err := network.GetWifiConfig(ctx)
			if err != nil {
				return nil, err
			}
			wifiSummary := make([]map[string]any, 0, len(wifi))
			for _, item := range wifi {
				wifiSummary = append(wifiSummary, map[string]any{"device": item.Device, "channel": item.Channel, "ssid": item.SSID, "encryption": item.Encryption, "mode": item.Mode, "disabled": item.Disabled})
			}
			return map[string]any{"interfaces": interfaces, "wifi": wifiSummary}, nil
		case "dhcp":
			return network.GetDHCPLeases(ctx)
		case "packages":
			return client.ListPackages(ctx)
		case "firewall":
			rules, err := firewall.ListRules(ctx)
			if err != nil {
				return nil, err
			}
			zones, err := firewall.ListZones(ctx)
			if err != nil {
				return nil, err
			}
			forwards, err := firewall.ListPortForwards(ctx)
			if err != nil {
				return nil, err
			}
			return map[string]any{"rules": rules, "zones": zones, "port_forwards": forwards}, nil
		case "services":
			return listOpenWrtServices(ctx, client), nil
		default:
			return nil, fmt.Errorf("unsupported OpenWrt section %q", section)
		}
	})
}

func listOpenWrtServices(ctx context.Context, client *openwrt.Client) []any {
	names := []string{"network", "firewall", "dnsmasq", "dropbear", "uhttpd", "odhcpd"}
	services := make([]any, 0, len(names))
	for _, name := range names {
		if ctx.Err() != nil {
			break
		}
		status, err := client.GetServiceStatus(ctx, name)
		if err == nil {
			services = append(services, map[string]any{"name": name, "running": status.Running, "enabled": status.Enabled, "instances": status.Instances, "triggers": status.Triggers})
		}
	}
	return services
}

func (s *LocalService) TestOpenWrt(ctx context.Context, id int64) error {
	_, err := s.withOpenWrt(ctx, id, func(client *openwrt.Client) (any, error) { return client.GetSystemInfo(ctx) })
	return err
}

func (s *LocalService) ActOpenWrt(ctx context.Context, id int64, action, service string) error {
	_, err := s.withOpenWrt(ctx, id, func(client *openwrt.Client) (any, error) {
		switch action {
		case "reboot":
			return nil, client.Reboot(ctx)
		case "restart_service":
			switch service {
			case "network", "firewall", "dnsmasq", "dropbear", "uhttpd", "odhcpd":
			default:
				return nil, fmt.Errorf("unsupported OpenWrt service %q", service)
			}
			return nil, client.RestartService(ctx, service)
		default:
			return nil, fmt.Errorf("unsupported OpenWrt action %q", action)
		}
	})
	return err
}

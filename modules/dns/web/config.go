package web

import (
	"bufio"
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"

	dnsconfig "github.com/lsongdev/dns-go/config"
	dnsmodule "github.com/lsongdev/ohmyserver/modules/dns"
)

const maxSubscriptionSize = 32 << 20

// BuildConfig assembles the DNS listener, local zone, upstream, and filter
// configuration from DNS-owned persistence. The host decides whether to run it.
func BuildConfig(ctx context.Context, repo *Repository, listDir string, refresh bool) (*dnsconfig.Config, bool, error) {
	settings, err := repo.GetSettings(ctx)
	if err != nil {
		return nil, false, err
	}
	cfg := &dnsconfig.Config{
		Cache: dnsconfig.CacheSpec{Enabled: settings.CacheEnabled, MinTTL: dnsconfig.Duration(time.Duration(settings.CacheMinTTL) * time.Second), MaxTTL: dnsconfig.Duration(time.Duration(settings.CacheMaxTTL) * time.Second), NegativeTTL: dnsconfig.Duration(60 * time.Second), MaxEntries: settings.CacheEntries},
		Proxy: dnsconfig.ProxySpec{Strategy: "failover"},
	}
	if settings.Enabled {
		for _, listen := range []struct{ kind, addr string }{{"udp", settings.UDPAddr}, {"tcp", settings.TCPAddr}, {"doh", settings.DoHAddr}} {
			if strings.TrimSpace(listen.addr) != "" {
				cfg.Listens = append(cfg.Listens, dnsconfig.ListenSpec{Type: listen.kind, Addr: listen.addr})
			}
		}
		if strings.TrimSpace(settings.DoTAddr) != "" {
			cfg.Listens = append(cfg.Listens, dnsconfig.ListenSpec{Type: "dot", Addr: settings.DoTAddr, CertFile: settings.CertFile, KeyFile: settings.KeyFile})
		}
	}
	cfg.Proxy.Upstreams, err = ParseUpstreams(settings.Upstreams)
	if err != nil {
		return nil, false, err
	}
	zones, err := repo.Zones(ctx)
	if err != nil {
		return nil, false, err
	}
	for _, zone := range zones {
		if !zone.Enabled {
			continue
		}
		records, err := repo.RecordsByZone(ctx, zone.ID)
		if err != nil {
			return nil, false, err
		}
		spec := dnsconfig.DomainSpec{Domain: zone.Name}
		for _, record := range records {
			if record.Enabled {
				spec.Records = append(spec.Records, RecordLine(record))
			}
		}
		cfg.Domains = append(cfg.Domains, spec)
	}
	subscriptions, err := repo.Subscriptions(ctx)
	if err != nil {
		return nil, false, err
	}
	for i := range subscriptions {
		item := &subscriptions[i]
		if !item.Enabled {
			continue
		}
		due := item.LastUpdated == nil || time.Since(*item.LastUpdated) >= time.Duration(item.RefreshHours)*time.Hour
		if refresh && settings.Enabled && (due || item.LocalPath == "") {
			if err := RefreshSubscription(ctx, repo, listDir, item); err != nil {
				if item.LocalPath == "" {
					continue
				}
			} else if current, getErr := repo.Subscription(ctx, item.ID); getErr == nil {
				item = current
			}
		}
		if item.LocalPath == "" {
			continue
		}
		spec := dnsconfig.ListSpec{Name: item.Name, URL: item.URL, File: item.LocalPath, Enabled: true, Refresh: dnsconfig.Duration(time.Duration(item.RefreshHours) * time.Hour)}
		if item.Kind == "allow" {
			cfg.Filters.Allowlists = append(cfg.Filters.Allowlists, spec)
		} else {
			cfg.Filters.Blocklists = append(cfg.Filters.Blocklists, spec)
		}
	}
	rules, err := repo.Rules(ctx)
	if err != nil {
		return nil, false, err
	}
	for _, rule := range rules {
		if rule.Enabled {
			cfg.Filters.Rules = append(cfg.Filters.Rules, rule.Rule)
		}
	}
	if settings.Enabled {
		if err := cfg.Validate(); err != nil {
			return nil, false, err
		}
	}
	return cfg, settings.Enabled, nil
}

func ParseUpstreams(value string) ([]dnsconfig.UpstreamSpec, error) {
	var upstreams []dnsconfig.UpstreamSpec
	for lineNo, line := range splitLines(value) {
		fields := strings.Fields(line)
		if len(fields) < 2 {
			return nil, fmt.Errorf("upstream line %d must be: type address [timeout] [method]", lineNo+1)
		}
		upstream := dnsconfig.UpstreamSpec{Type: strings.ToLower(fields[0]), Addr: fields[1], Timeout: dnsconfig.Duration(5 * time.Second)}
		if len(fields) > 2 {
			timeout, err := time.ParseDuration(fields[2])
			if err != nil {
				return nil, fmt.Errorf("upstream line %d timeout: %w", lineNo+1, err)
			}
			upstream.Timeout = dnsconfig.Duration(timeout)
		}
		if len(fields) > 3 {
			upstream.Method = strings.ToLower(fields[3])
		} else if upstream.Type == "doh" {
			upstream.Method = "post"
		}
		upstreams = append(upstreams, upstream)
	}
	return upstreams, nil
}

func RefreshSubscription(ctx context.Context, repo *Repository, listDir string, item *dnsmodule.Subscription) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	parsed, err := url.ParseRequestURI(item.URL)
	if err != nil || (parsed.Scheme != "http" && parsed.Scheme != "https") {
		return fmt.Errorf("subscription URL must use http or https")
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, item.URL, nil)
	if err != nil {
		return err
	}
	req.Header.Set("User-Agent", "OhMyServer DNS/1.0")
	client := &http.Client{Timeout: 45 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		persistSubscriptionError(ctx, repo, item.ID, err)
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		err = fmt.Errorf("subscription returned %s", resp.Status)
		persistSubscriptionError(ctx, repo, item.ID, err)
		return err
	}
	if err := os.MkdirAll(listDir, 0755); err != nil {
		return err
	}
	path := filepath.Join(listDir, fmt.Sprintf("%d.txt", item.ID))
	temp, err := os.CreateTemp(listDir, fmt.Sprintf("%d-*.tmp", item.ID))
	if err != nil {
		return err
	}
	tempPath := temp.Name()
	defer os.Remove(tempPath)
	n, copyErr := copyLimited(ctx, temp, resp.Body, maxSubscriptionSize+1)
	closeErr := temp.Close()
	if copyErr != nil {
		return copyErr
	}
	if closeErr != nil {
		return closeErr
	}
	if n > maxSubscriptionSize {
		return fmt.Errorf("subscription exceeds the 32 MiB limit")
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	if err := os.Rename(tempPath, path); err != nil {
		return err
	}
	if err := repo.SetSubscriptionRefresh(ctx, item.ID, path, time.Now()); err != nil {
		return err
	}
	now := time.Now()
	item.LocalPath, item.LastUpdated = path, &now
	return nil
}

func copyLimited(ctx context.Context, dst *os.File, src io.Reader, limit int64) (int64, error) {
	buffer := make([]byte, 32*1024)
	var total int64
	for {
		if err := ctx.Err(); err != nil {
			return total, err
		}
		count, readErr := src.Read(buffer)
		if count > 0 {
			total += int64(count)
			if total > limit {
				return total, nil
			}
			if _, err := dst.Write(buffer[:count]); err != nil {
				return total, err
			}
		}
		if readErr != nil {
			if errors.Is(readErr, io.EOF) {
				return total, nil
			}
			return total, readErr
		}
	}
}

func persistSubscriptionError(parent context.Context, repo *Repository, id int64, refreshErr error) {
	ctx, cancel := context.WithTimeout(context.WithoutCancel(parent), 5*time.Second)
	defer cancel()
	_ = repo.SetSubscriptionError(ctx, id, refreshErr.Error())
}

func RemoveSubscriptionFile(path string) error {
	if strings.TrimSpace(path) == "" {
		return nil
	}
	err := os.Remove(path)
	if os.IsNotExist(err) {
		return nil
	}
	return err
}

func RecordLine(record dnsmodule.LocalRecord) string {
	value := record.Value
	switch record.Type {
	case "MX":
		value = fmt.Sprintf("%d %s", record.Priority, value)
	case "SRV":
		value = fmt.Sprintf("%d %d %d %s", record.Priority, record.Weight, record.Port, value)
	case "TXT":
		value = `"` + strings.ReplaceAll(value, `"`, `\"`) + `"`
	}
	return fmt.Sprintf("%s %d IN %s %s", record.Name, record.TTL, record.Type, value)
}

func splitLines(value string) []string {
	var lines []string
	scanner := bufio.NewScanner(strings.NewReader(value))
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line != "" && !strings.HasPrefix(line, "#") {
			lines = append(lines, line)
		}
	}
	return lines
}

// NormalizeSettings applies DNS-specific defaults and validates the config
// shape before either host or standalone lifecycle policy persists it.
func NormalizeSettings(settings dnsmodule.Settings) (dnsmodule.Settings, error) {
	settings.UDPAddr, settings.TCPAddr = strings.TrimSpace(settings.UDPAddr), strings.TrimSpace(settings.TCPAddr)
	settings.DoTAddr, settings.DoHAddr = strings.TrimSpace(settings.DoTAddr), strings.TrimSpace(settings.DoHAddr)
	settings.CertFile, settings.KeyFile = strings.TrimSpace(settings.CertFile), strings.TrimSpace(settings.KeyFile)
	settings.Upstreams = strings.TrimSpace(settings.Upstreams)
	if settings.Enabled && settings.UDPAddr == "" && settings.TCPAddr == "" && settings.DoTAddr == "" && settings.DoHAddr == "" {
		return settings, fmt.Errorf("at least one DNS listen address is required")
	}
	if settings.CacheMinTTL < 0 || settings.CacheMaxTTL <= 0 || settings.CacheMinTTL > settings.CacheMaxTTL {
		return settings, fmt.Errorf("cache TTL values are invalid")
	}
	if settings.CacheEntries <= 0 {
		return settings, fmt.Errorf("cache entries must be greater than zero")
	}
	if settings.DoTAddr != "" {
		if _, err := tls.LoadX509KeyPair(settings.CertFile, settings.KeyFile); err != nil {
			return settings, fmt.Errorf("invalid DNS-over-TLS certificate: %w", err)
		}
	}
	upstreams, err := ParseUpstreams(settings.Upstreams)
	if err != nil {
		return settings, err
	}
	candidate := &dnsconfig.Config{Proxy: dnsconfig.ProxySpec{Strategy: "failover", Upstreams: upstreams}}
	if settings.Enabled {
		if settings.UDPAddr != "" {
			candidate.Listens = append(candidate.Listens, dnsconfig.ListenSpec{Type: "udp", Addr: settings.UDPAddr})
		}
		if settings.TCPAddr != "" {
			candidate.Listens = append(candidate.Listens, dnsconfig.ListenSpec{Type: "tcp", Addr: settings.TCPAddr})
		}
		if settings.DoTAddr != "" {
			candidate.Listens = append(candidate.Listens, dnsconfig.ListenSpec{Type: "dot", Addr: settings.DoTAddr, CertFile: settings.CertFile, KeyFile: settings.KeyFile})
		}
		if settings.DoHAddr != "" {
			candidate.Listens = append(candidate.Listens, dnsconfig.ListenSpec{Type: "doh", Addr: settings.DoHAddr})
		}
		if err := candidate.Validate(); err != nil {
			return settings, err
		}
	}
	return settings, nil
}

package web

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	sshmodule "github.com/lsongdev/ohmyserver/modules/ssh"
)

type LocalService struct{ repo *Repository }

func NewLocalService(repo *Repository) *LocalService { return &LocalService{repo: repo} }
func summary(p Profile) sshmodule.Server {
	return sshmodule.Server{ID: p.ID, Name: p.Name, Host: p.Host, Port: p.Port, Username: p.Username, AuthType: p.AuthType, HostKey: p.HostKey, Description: p.Description, Enabled: p.Enabled, HasCredential: p.Password != "" || p.PrivateKey != ""}
}
func (s *LocalService) ListSSHProfiles(ctx context.Context) ([]sshmodule.Server, error) {
	ps, e := s.repo.List(ctx)
	if e != nil {
		return nil, e
	}
	out := make([]sshmodule.Server, 0, len(ps))
	for _, p := range ps {
		out = append(out, summary(p))
	}
	return out, nil
}
func (s *LocalService) GetSSHProfile(ctx context.Context, id int64) (sshmodule.Server, error) {
	p, e := s.repo.Get(ctx, id)
	if e != nil {
		return sshmodule.Server{}, e
	}
	return summary(p), nil
}
func (s *LocalService) SaveSSHProfile(ctx context.Context, id int64, in sshmodule.Input) (sshmodule.Server, error) {
	var p Profile
	var e error
	if id != 0 {
		p, e = s.repo.Get(ctx, id)
		if e != nil {
			return sshmodule.Server{}, e
		}
		if in.AuthType == p.AuthType {
			if in.AuthType == "password" && in.Password == "" {
				in.Password = p.Password
			}
			if in.AuthType == "key" && in.PrivateKey == "" {
				in.PrivateKey = p.PrivateKey
			}
		}
	} else {
		p.Enabled = true
	}
	if e = sshmodule.Validate(&in); e != nil {
		return sshmodule.Server{}, e
	}
	oldHost, oldPort := p.Host, p.Port
	p.Name, p.Host, p.Port, p.Username, p.AuthType, p.Description = in.Name, in.Host, in.Port, in.Username, in.AuthType, in.Description
	if in.AuthType == "password" {
		p.Password, p.PrivateKey = in.Password, ""
	} else {
		p.PrivateKey, p.Password = in.PrivateKey, ""
	}
	if in.Enabled != nil {
		p.Enabled = *in.Enabled
	}
	if oldHost != p.Host || oldPort != p.Port {
		p.HostKey = ""
	}
	if id == 0 {
		e = s.repo.Create(ctx, &p)
	} else {
		e = s.repo.Update(ctx, &p)
	}
	if e != nil {
		return sshmodule.Server{}, e
	}
	return summary(p), nil
}
func (s *LocalService) DeleteSSHProfile(ctx context.Context, id int64) error {
	return s.repo.Delete(ctx, id)
}
func (s *LocalService) TestSSHProfile(ctx context.Context, in sshmodule.Input) error {
	if e := sshmodule.Validate(&in); e != nil {
		return e
	}
	p := Profile{Name: in.Name, Host: in.Host, Port: in.Port, Username: in.Username, AuthType: in.AuthType, Password: in.Password, PrivateKey: in.PrivateKey}
	c, e := sshmodule.Connect(ctx, p)
	if e != nil {
		return errors.Join(sshmodule.ErrConnection, e)
	}
	return c.Close()
}
func (s *LocalService) connect(ctx context.Context, p Profile) (*sshmodule.Client, error) {
	c, e := sshmodule.Connect(ctx, p)
	if e != nil {
		return nil, errors.Join(sshmodule.ErrConnection, e)
	}
	if p.ID != 0 && p.HostKey == "" && c.ObservedHostKey != "" {
		if e = s.repo.SetHostKey(ctx, p.ID, c.ObservedHostKey); e != nil {
			_ = c.Close()
			return nil, fmt.Errorf("pin SSH host key: %w", e)
		}
	}
	return c, nil
}
func (s *LocalService) profileMetrics(ctx context.Context, id int64) (sshmodule.Metrics, error) {
	p, e := s.repo.Get(ctx, id)
	if e != nil {
		return sshmodule.Metrics{}, e
	}
	return s.CollectProfileMetrics(ctx, p)
}
func (s *LocalService) CollectProfileMetrics(parent context.Context, p Profile) (sshmodule.Metrics, error) {
	ctx, cancel := context.WithTimeout(parent, 15*time.Second)
	defer cancel()
	if !p.Enabled {
		return sshmodule.Metrics{}, sshmodule.ErrDisabled
	}
	c, e := s.connect(ctx, p)
	if e != nil {
		return sshmodule.Metrics{}, e
	}
	defer c.Close()
	out, e := c.Run(ctx, remoteMetricsCommand)
	if e != nil {
		return sshmodule.Metrics{}, errors.Join(sshmodule.ErrConnection, e)
	}
	raw := string(out)
	a, b := strings.Index(raw, "{"), strings.LastIndex(raw, "}")
	if a < 0 || b <= a {
		return sshmodule.Metrics{}, fmt.Errorf("no JSON in SSH metrics output")
	}
	var v map[string]float64
	if e = json.Unmarshal([]byte(raw[a:b+1]), &v); e != nil {
		return sshmodule.Metrics{}, fmt.Errorf("parse SSH metrics: %w", e)
	}
	m := sshmodule.Metrics{Host: p.Name, CPUUsage: v["cpu"], CPUCores: int(v["cores"]), MemoryTotal: uint64(v["mem_total"]), MemoryUsed: uint64(v["mem_used"]), MemoryFree: uint64(v["mem_free"]), DiskTotal: uint64(v["disk_total"]), DiskUsed: uint64(v["disk_used"]), DiskFree: uint64(v["disk_free"]), NetworkReceived: uint64(v["net_rx"]), NetworkSent: uint64(v["net_tx"]), Uptime: uint64(v["uptime"]), Load1: v["load1"], Load5: v["load5"], Load15: v["load15"]}
	if m.MemoryTotal > 0 {
		m.MemoryUsage = float64(m.MemoryUsed) / float64(m.MemoryTotal) * 100
	}
	if m.DiskTotal > 0 {
		m.DiskUsage = float64(m.DiskUsed) / float64(m.DiskTotal) * 100
	}
	return m, nil
}
func (s *LocalService) OpenTerminal(ctx context.Context, id int64, cols, rows uint16) (sshmodule.TerminalSession, error) {
	p, err := s.repo.Get(ctx, id)
	if err != nil {
		return nil, err
	}
	if !p.Enabled {
		return nil, sshmodule.ErrDisabled
	}
	ctx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()
	c, err := s.connect(ctx, p)
	if err != nil {
		return nil, err
	}
	session, err := c.OpenTerminal(cols, rows)
	if err != nil {
		_ = c.Close()
		return nil, err
	}
	return session, nil
}
func (s *LocalService) CollectEnabledMetrics(ctx context.Context) ([]sshmodule.Metrics, error) {
	profiles, err := s.repo.List(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]sshmodule.Metrics, 0, len(profiles))
	var failures []error
	for _, p := range profiles {
		if !p.Enabled {
			continue
		}
		m, e := s.CollectProfileMetrics(ctx, p)
		if e != nil {
			failures = append(failures, fmt.Errorf("%s: %w", p.Name, e))
			continue
		}
		out = append(out, m)
	}
	return out, errors.Join(failures...)
}
func (s *LocalService) SSHStats(ctx context.Context, id int64) (sshmodule.Stats, error) {
	m, e := s.profileMetrics(ctx, id)
	if e != nil {
		return sshmodule.Stats{}, e
	}
	return sshmodule.Stats{Connected: true, ConnectedAt: time.Now(), CPUPercent: m.CPUUsage, MemoryPercent: m.MemoryUsage, MemoryUsage: m.MemoryUsed, MemoryTotal: m.MemoryTotal, NetworkRX: m.NetworkReceived, NetworkTX: m.NetworkSent, LoadAverage: []float64{m.Load1, m.Load5, m.Load15}}, nil
}
func (s *LocalService) SSHMetrics(ctx context.Context, id int64) (sshmodule.Metrics, error) {
	return s.profileMetrics(ctx, id)
}

const remoteMetricsCommand = `cpu=$(top -bn1 | grep "Cpu(s)" | awk '{print $2}' | cut -d'%' -f1 2>/dev/null || echo ""); if [ -z "$cpu" ]; then cpu=$(vmstat 1 2 | tail -1 | awk '{print 100 - $15}' 2>/dev/null || echo "0"); fi; mem=$(free -b | awk 'NR==2{printf "%.0f %.0f %.0f", $2, $3, $2-$3}' 2>/dev/null || echo "0 0 0"); disk=$(df -B1 / 2>/dev/null | awk 'NR==2{printf "%.0f %.0f %.0f", $2, $3, $4}' || echo "0 0 0"); net=$(cat /proc/net/dev 2>/dev/null | grep -E "^\s*(eth0|ens|enp|wlan|wl)" | head -1 | awk '{print $2, $10}' || echo "0 0"); load=$(cat /proc/loadavg 2>/dev/null | awk '{print $1, $2, $3}' || echo "0 0 0"); uptime=$(cat /proc/uptime 2>/dev/null | awk '{print $1}' || echo "0"); cores=$(nproc 2>/dev/null || echo "1"); echo "{\"cpu\":$cpu,\"mem_total\":$(echo $mem|awk '{print $1}'),\"mem_used\":$(echo $mem|awk '{print $2}'),\"mem_free\":$(echo $mem|awk '{print $3}'),\"disk_total\":$(echo $disk|awk '{print $1}'),\"disk_used\":$(echo $disk|awk '{print $2}'),\"disk_free\":$(echo $disk|awk '{print $3}'),\"net_rx\":$(echo $net|awk '{print $1}'),\"net_tx\":$(echo $net|awk '{print $2}'),\"load1\":$(echo $load|awk '{print $1}'),\"load5\":$(echo $load|awk '{print $2}'),\"load15\":$(echo $load|awk '{print $3}'),\"uptime\":$uptime,\"cores\":$cores}"`

var _ sshmodule.Service = (*LocalService)(nil)

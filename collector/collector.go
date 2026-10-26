// Package collector defines where readings come from. monitor-go ships
// Local, which reads this machine; a host program adds others (for example
// SSH hosts read through procfs) by implementing Collector.
package collector

import (
	"context"
	"os"
	"runtime"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/shirou/gopsutil/v4/cpu"
	"github.com/shirou/gopsutil/v4/disk"
	"github.com/shirou/gopsutil/v4/host"
	"github.com/shirou/gopsutil/v4/load"
	"github.com/shirou/gopsutil/v4/mem"
	psnet "github.com/shirou/gopsutil/v4/net"

	"github.com/lsongdev/monitor-go/metrics"
)

// Host is a machine a collector can read. IDs are "<kind>" or
// "<kind>:<name>", e.g. "local" or "ssh:3", and are stored with its
// history.
type Host struct {
	ID   string `json:"id"`
	Name string `json:"name"`
	Kind string `json:"kind"`
}

// Kind returns the kind part of a host ID.
func Kind(host string) string {
	kind, _, _ := strings.Cut(host, ":")
	return kind
}

// Collector reads the hosts of one kind.
type Collector interface {
	Kind() string
	// Hosts lists the hosts this collector can read now.
	Hosts(ctx context.Context) ([]Host, error)
	// Read takes a reading of host, with its info.
	Read(ctx context.Context, host string) (metrics.Reading, metrics.Info, error)
}

// Local reads this machine.
type Local struct {
	mu       sync.Mutex
	info     metrics.Info
	infoTime time.Time
}

func (*Local) Kind() string { return "local" }

func (l *Local) Hosts(context.Context) ([]Host, error) {
	name, _ := os.Hostname()
	if name == "" {
		name = "localhost"
	}
	return []Host{{ID: "local", Name: name, Kind: "local"}}, nil
}

func (l *Local) Read(ctx context.Context, _ string) (metrics.Reading, metrics.Info, error) {
	r := metrics.Reading{Time: time.Now()}
	times, err := cpu.TimesWithContext(ctx, false)
	if err != nil {
		return r, metrics.Info{}, err
	}
	if len(times) > 0 {
		t := times[0]
		idle := t.Idle + t.Iowait
		total := t.User + t.Nice + t.System + t.Idle + t.Iowait + t.Irq + t.Softirq + t.Steal
		r.CPUTotal, r.CPUBusy = total, total-idle
	}
	if avg, err := load.AvgWithContext(ctx); err == nil {
		r.Load1, r.Load5, r.Load15 = avg.Load1, avg.Load5, avg.Load15
	}
	if vm, err := mem.VirtualMemoryWithContext(ctx); err == nil {
		r.MemTotal = vm.Total
		if vm.Total > vm.Available {
			r.MemUsed = vm.Total - vm.Available
		}
	}
	if sw, err := mem.SwapMemoryWithContext(ctx); err == nil {
		r.SwapTotal, r.SwapUsed = sw.Total, sw.Used
	}
	r.Disks = localDisks(ctx)
	if io, err := disk.IOCountersWithContext(ctx); err == nil {
		for name, c := range io {
			if metrics.WholeDisk(name) {
				r.DiskRead += c.ReadBytes
				r.DiskWrite += c.WriteBytes
			}
		}
	}
	if ifaces, err := psnet.IOCountersWithContext(ctx, true); err == nil {
		for _, c := range ifaces {
			if metrics.PhysicalInterface(c.Name) {
				r.NetRecv += c.BytesRecv
				r.NetSent += c.BytesSent
			}
		}
	}
	if up, err := host.UptimeWithContext(ctx); err == nil {
		r.Uptime = time.Duration(up) * time.Second
	}
	return r, l.hostInfo(ctx), nil
}

func localDisks(ctx context.Context) []metrics.Disk {
	parts, err := disk.PartitionsWithContext(ctx, false)
	if err != nil {
		return nil
	}
	var disks []metrics.Disk
	hasData := false
	for _, p := range parts {
		// Read-only mounts are disk images and system snapshots, not
		// storage that fills up.
		if !metrics.RealFS(p.Device, p.Fstype, p.Mountpoint) || slices.Contains(p.Opts, "ro") && p.Mountpoint != "/" {
			continue
		}
		u, err := disk.UsageWithContext(ctx, p.Mountpoint)
		if err != nil || u.Total == 0 {
			continue
		}
		hasData = hasData || p.Mountpoint == "/System/Volumes/Data"
		disks = append(disks, metrics.Disk{Mount: p.Mountpoint, Device: p.Device, FSType: p.Fstype, Total: u.Total, Used: u.Used})
	}
	// On macOS "/" is a read-only snapshot in the same APFS container as
	// the Data volume, which holds what users store: report Data alone.
	if runtime.GOOS == "darwin" && hasData {
		kept := disks[:0]
		for _, d := range disks {
			if d.Mount != "/" {
				kept = append(kept, d)
			}
		}
		disks = kept
	}
	return metrics.DedupeDisks(disks)
}

// hostInfo is re-read every ten minutes: it rarely changes and some of it
// is slow to gather.
func (l *Local) hostInfo(ctx context.Context) metrics.Info {
	l.mu.Lock()
	defer l.mu.Unlock()
	if !l.infoTime.IsZero() && time.Since(l.infoTime) < 10*time.Minute {
		return l.info
	}
	info := metrics.Info{Arch: runtime.GOARCH, CPUCores: runtime.NumCPU()}
	if h, err := host.InfoWithContext(ctx); err == nil {
		info.Hostname, info.Kernel = h.Hostname, h.KernelVersion
		info.OS = strings.TrimSpace(h.Platform + " " + h.PlatformVersion)
		if h.Platform == "darwin" {
			info.OS = "macOS " + h.PlatformVersion
		}
		if h.KernelArch != "" {
			info.Arch = h.KernelArch
		}
		info.Kernel = strings.TrimSpace(capitalize(h.OS) + " " + h.KernelVersion)
	}
	if c, err := cpu.InfoWithContext(ctx); err == nil && len(c) > 0 {
		info.CPUModel = strings.TrimSpace(c[0].ModelName)
	}
	// IPv4 addresses, then at most two IPv6 ones: hosts with privacy
	// extensions carry many temporary IPv6 addresses.
	if ifaces, err := psnet.InterfacesWithContext(ctx); err == nil {
		var v6 []string
		for _, iface := range ifaces {
			if !metrics.PhysicalInterface(iface.Name) {
				continue
			}
			for _, a := range iface.Addrs {
				addr, _, _ := strings.Cut(a.Addr, "/")
				switch {
				case strings.HasPrefix(addr, "fe80:"), strings.HasPrefix(addr, "127."), addr == "::1":
				case strings.Contains(addr, ":"):
					v6 = append(v6, addr)
				default:
					info.Addresses = append(info.Addresses, addr)
				}
			}
		}
		info.Addresses = append(info.Addresses, v6[:min(len(v6), 2)]...)
	}
	l.info, l.infoTime = info, time.Now()
	return info
}

func capitalize(s string) string {
	switch s {
	case "linux":
		return "Linux"
	case "darwin":
		return "Darwin"
	case "freebsd":
		return "FreeBSD"
	}
	return s
}

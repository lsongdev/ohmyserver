// Package procfs reads a Linux host through a shell: Script prints what
// /proc and a few commands say, in one round trip, and Parse turns that
// output into a reading and host info. It needs only a POSIX shell, cat,
// df and uname on the host (busybox is enough), so a host program can
// monitor any Linux machine it can run a command on, e.g. over SSH,
// without installing anything there.
package procfs

import (
	"bufio"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/lsongdev/monitor-go/metrics"
)

// Script prints the sections Parse reads. LC_ALL=C keeps df's output
// parseable.
const Script = `export LC_ALL=C
echo @stat; head -n 1 /proc/stat
echo @loadavg; cat /proc/loadavg
echo @meminfo; cat /proc/meminfo
echo @netdev; cat /proc/net/dev
echo @diskstats; cat /proc/diskstats
echo @mounts; cat /proc/mounts
echo @df; df -kP 2>/dev/null
echo @uptime; cat /proc/uptime
echo @hostname; cat /proc/sys/kernel/hostname
echo @uname; uname -srm
echo @cpuinfo; grep -m 1 -i -E '^(model name|hardware|processor)' /proc/cpuinfo; grep -c '^processor' /proc/cpuinfo
echo @osrelease; cat /etc/os-release 2>/dev/null
echo @addr; hostname -I 2>/dev/null || ip -o -4 addr show scope global 2>/dev/null | awk '{print $4}'
echo @end`

// ErrNotLinux reports output that does not come from a Linux host.
var ErrNotLinux = errors.New("not a Linux host: /proc is unavailable")

// Parse reads Script's output, taken at time now.
func Parse(output string, now time.Time) (metrics.Reading, metrics.Info, error) {
	sections := map[string][]string{}
	var name string
	scanner := bufio.NewScanner(strings.NewReader(output))
	scanner.Buffer(make([]byte, 64<<10), 4<<20)
	for scanner.Scan() {
		line := scanner.Text()
		if strings.HasPrefix(line, "@") && !strings.Contains(line, " ") {
			name = line[1:]
			sections[name] = []string{}
			continue
		}
		if name != "" {
			sections[name] = append(sections[name], line)
		}
	}
	r := metrics.Reading{Time: now}
	var info metrics.Info
	if _, ok := sections["end"]; !ok {
		return r, info, errors.New("incomplete output")
	}
	if len(sections["stat"]) == 0 || !strings.HasPrefix(sections["stat"][0], "cpu ") {
		return r, info, ErrNotLinux
	}
	parseStat(sections["stat"][0], &r)
	if r.CPUTotal == 0 {
		return r, info, fmt.Errorf("%w (no CPU figures)", ErrNotLinux)
	}
	if f := strings.Fields(first(sections["loadavg"])); len(f) >= 3 {
		r.Load1, r.Load5, r.Load15 = num(f[0]), num(f[1]), num(f[2])
	}
	parseMeminfo(sections["meminfo"], &r)
	parseNetDev(sections["netdev"], &r)
	parseDiskstats(sections["diskstats"], &r)
	parseDf(sections["df"], fsTypes(sections["mounts"]), &r)
	if f := strings.Fields(first(sections["uptime"])); len(f) > 0 {
		r.Uptime = time.Duration(num(f[0]) * float64(time.Second))
	}

	info.Hostname = strings.TrimSpace(first(sections["hostname"]))
	if f := strings.Fields(first(sections["uname"])); len(f) >= 3 {
		info.Kernel, info.Arch = f[0]+" "+f[1], f[2]
	}
	for _, line := range sections["cpuinfo"] {
		if k, v, ok := strings.Cut(line, ":"); ok {
			if model := strings.TrimSpace(v); info.CPUModel == "" && strings.TrimSpace(k) != "processor" {
				info.CPUModel = model
			}
		} else if n, err := strconv.Atoi(strings.TrimSpace(line)); err == nil {
			info.CPUCores = n
		}
	}
	for _, line := range sections["osrelease"] {
		if v, ok := strings.CutPrefix(line, "PRETTY_NAME="); ok {
			info.OS = strings.Trim(v, `"'`)
		}
	}
	if info.OS == "" {
		info.OS = "Linux"
	}
	for _, line := range sections["addr"] {
		for _, a := range strings.Fields(line) {
			a, _, _ = strings.Cut(a, "/")
			if strings.Contains(a, ".") || strings.Count(a, ":") >= 2 {
				info.Addresses = append(info.Addresses, a)
			}
		}
	}
	return r, info, nil
}

func first(lines []string) string {
	if len(lines) == 0 {
		return ""
	}
	return lines[0]
}

func num(s string) float64 {
	v, _ := strconv.ParseFloat(s, 64)
	return v
}

func unum(s string) uint64 {
	v, _ := strconv.ParseUint(s, 10, 64)
	return v
}

// parseStat reads "cpu user nice system idle iowait irq softirq steal ...".
// Idle and iowait are idle time; guest time is already counted in user.
func parseStat(line string, r *metrics.Reading) {
	f := strings.Fields(line)[1:]
	var total float64
	for i, v := range f {
		if i >= 8 { // guest, guest_nice
			break
		}
		total += num(v)
	}
	idle := 0.0
	if len(f) > 3 {
		idle += num(f[3])
	}
	if len(f) > 4 {
		idle += num(f[4])
	}
	r.CPUTotal, r.CPUBusy = total, total-idle
}

func parseMeminfo(lines []string, r *metrics.Reading) {
	kb := map[string]uint64{}
	for _, line := range lines {
		k, v, ok := strings.Cut(line, ":")
		if !ok {
			continue
		}
		if f := strings.Fields(v); len(f) > 0 {
			kb[k] = unum(f[0]) * 1024
		}
	}
	r.MemTotal = kb["MemTotal"]
	available, ok := kb["MemAvailable"]
	if !ok { // kernels before 3.14
		available = kb["MemFree"] + kb["Buffers"] + kb["Cached"]
	}
	if r.MemTotal > available {
		r.MemUsed = r.MemTotal - available
	}
	r.SwapTotal = kb["SwapTotal"]
	if r.SwapTotal > kb["SwapFree"] {
		r.SwapUsed = r.SwapTotal - kb["SwapFree"]
	}
}

// parseNetDev reads "iface: rx_bytes ... (8 rx fields) tx_bytes ...".
func parseNetDev(lines []string, r *metrics.Reading) {
	for _, line := range lines {
		name, rest, ok := strings.Cut(line, ":")
		name = strings.TrimSpace(name)
		if !ok || !metrics.PhysicalInterface(name) {
			continue
		}
		if f := strings.Fields(rest); len(f) >= 9 {
			r.NetRecv += unum(f[0])
			r.NetSent += unum(f[8])
		}
	}
}

// parseDiskstats reads "major minor name reads merged sectors_read ...";
// sectors are 512 bytes whatever the device's block size.
func parseDiskstats(lines []string, r *metrics.Reading) {
	for _, line := range lines {
		f := strings.Fields(line)
		if len(f) < 10 || !metrics.WholeDisk(f[2]) {
			continue
		}
		r.DiskRead += unum(f[5]) * 512
		r.DiskWrite += unum(f[9]) * 512
	}
}

// mountEscapes undoes /proc/mounts' octal escapes of space, tab, newline
// and backslash.
var mountEscapes = strings.NewReplacer(`\040`, " ", `\011`, "\t", `\012`, "\n", `\134`, `\`)

func fsTypes(mounts []string) map[string]string {
	types := map[string]string{}
	for _, line := range mounts {
		if f := strings.Fields(line); len(f) >= 3 {
			types[mountEscapes.Replace(f[1])] = f[2]
		}
	}
	return types
}

// parseDf reads POSIX df output: device 1024-blocks used available
// capacity mount. Without any device-backed file system (a container)
// the root file system is reported.
func parseDf(lines []string, types map[string]string, r *metrics.Reading) {
	var root *metrics.Disk
	for i, line := range lines {
		f := strings.Fields(line)
		if i == 0 || len(f) < 6 {
			continue // header
		}
		mount := strings.Join(f[5:], " ")
		d := metrics.Disk{Mount: mount, Device: f[0], FSType: types[mount], Total: unum(f[1]) * 1024, Used: unum(f[2]) * 1024}
		if mount == "/" {
			root = &d
		}
		if metrics.RealFS(f[0], types[mount], mount) {
			r.Disks = append(r.Disks, d)
		}
	}
	r.Disks = metrics.DedupeDisks(r.Disks)
	if len(r.Disks) == 0 && root != nil {
		r.Disks = []metrics.Disk{*root}
	}
}

package procfs

import (
	"os"
	"strings"
	"testing"
	"time"

	"github.com/lsongdev/monitor-go/metrics"
)

const sample = `@stat
cpu  100 0 50 800 50 0 0 0 10 0
@loadavg
0.50 0.40 0.30 1/200 1234
@meminfo
MemTotal:        2000000 kB
MemFree:          500000 kB
MemAvailable:    1500000 kB
SwapTotal:       1000000 kB
SwapFree:         750000 kB
@netdev
Inter-|   Receive                                                |  Transmit
 face |bytes    packets errs drop fifo frame compressed multicast|bytes    packets errs drop fifo colls carrier compressed
    lo:  999999     10    0    0    0     0          0         0   999999     10    0    0    0     0       0          0
  eth0:    1000     10    0    0    0     0          0         0     2000     10    0    0    0     0       0          0
veth9a:    5000     10    0    0    0     0          0         0     5000     10    0    0    0     0       0          0
@diskstats
   8       0 sda 100 0 2000 0 50 0 4000 0 0 0 0
   8       1 sda1 100 0 2000 0 50 0 4000 0 0 0 0
   7       0 loop0 1 0 8 0 0 0 0 0 0 0 0
@mounts
/dev/sda1 / ext4 rw 0 0
/dev/sda1 /srv ext4 rw 0 0
/dev/sdb1 /mnt/my\040data xfs rw 0 0
tmpfs /run tmpfs rw 0 0
@df
Filesystem     1024-blocks    Used Available Capacity Mounted on
/dev/sda1         10000000 4000000   6000000      40% /
/dev/sda1         10000000 4000000   6000000      40% /srv
/dev/sdb1          2000000  500000   1500000      25% /mnt/my data
tmpfs               100000       0    100000       0% /run
@uptime
3600.50 7000.00
@hostname
nas
@uname
Linux 6.8.0-45-generic x86_64
@cpuinfo
model name	: Intel(R) N100
4
@osrelease
NAME="Ubuntu"
PRETTY_NAME="Ubuntu 24.04.1 LTS"
@addr
192.168.1.10 fd00::10
@end
`

func TestParse(t *testing.T) {
	now := time.Unix(1000, 0)
	r, info, err := Parse(sample, now)
	if err != nil {
		t.Fatal(err)
	}
	// user nice system idle iowait irq softirq steal: total 1000, idle
	// 800+50; guest (10) is already part of user.
	if r.CPUTotal != 1000 || r.CPUBusy != 150 || r.Load5 != 0.4 {
		t.Errorf("cpu/load %+v", r)
	}
	if r.MemTotal != 2000000*1024 || r.MemUsed != 500000*1024 || r.SwapUsed != 250000*1024 {
		t.Errorf("memory %+v", r)
	}
	if r.NetRecv != 1000 || r.NetSent != 2000 || r.DiskRead != 2000*512 || r.DiskWrite != 4000*512 {
		t.Errorf("counters %+v", r)
	}
	if len(r.Disks) != 2 || r.Disks[0] != (metrics.Disk{Mount: "/", Device: "/dev/sda1", FSType: "ext4", Total: 10000000 * 1024, Used: 4000000 * 1024}) || r.Disks[1].Mount != "/mnt/my data" || r.Disks[1].FSType != "xfs" {
		t.Errorf("disks %+v", r.Disks)
	}
	if r.Uptime != 3600500*time.Millisecond {
		t.Errorf("uptime %v", r.Uptime)
	}
	want := metrics.Info{Hostname: "nas", OS: "Ubuntu 24.04.1 LTS", Kernel: "Linux 6.8.0-45-generic", Arch: "x86_64", CPUModel: "Intel(R) N100", CPUCores: 4, Addresses: []string{"192.168.1.10", "fd00::10"}}
	if strings.Join(info.Addresses, ",") != strings.Join(want.Addresses, ",") || info.Hostname != want.Hostname || info.OS != want.OS ||
		info.Kernel != want.Kernel || info.Arch != want.Arch || info.CPUModel != want.CPUModel || info.CPUCores != want.CPUCores {
		t.Errorf("info %+v", info)
	}
}

// Real output from a Debian and a busybox (Alpine) container.
func TestParseRealHosts(t *testing.T) {
	for _, name := range []string{"debian", "alpine"} {
		data, err := os.ReadFile("testdata/" + name + ".txt")
		if err != nil {
			t.Fatal(err)
		}
		r, info, err := Parse(string(data), time.Now())
		if err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		if r.CPUTotal == 0 || r.MemTotal == 0 || r.MemUsed == 0 || r.Uptime == 0 || len(r.Disks) != 1 || r.Disks[0].Mount != "/" {
			t.Errorf("%s: reading %+v", name, r)
		}
		if info.Hostname == "" || info.Kernel == "" || info.CPUCores == 0 || !strings.Contains(info.OS, map[string]string{"debian": "Debian", "alpine": "Alpine"}[name]) {
			t.Errorf("%s: info %+v", name, info)
		}
	}
}

func TestParseRejectsOtherSystems(t *testing.T) {
	if _, _, err := Parse("@stat\nhead: /proc/stat: No such file\n@end\n", time.Now()); err == nil {
		t.Fatal("accepted output without /proc")
	}
	if _, _, err := Parse("@stat\ncpu 1 2 3 4\n", time.Now()); err == nil {
		t.Fatal("accepted truncated output")
	}
}

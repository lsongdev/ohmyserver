package metrics

import (
	"testing"
	"time"
)

func TestBetween(t *testing.T) {
	t0 := time.Unix(1000, 0)
	prev := Reading{Time: t0, CPUBusy: 100, CPUTotal: 400, DiskRead: 1000, NetRecv: 5000, NetSent: 10}
	cur := Reading{Time: t0.Add(10 * time.Second), CPUBusy: 150, CPUTotal: 500, DiskRead: 21000, NetRecv: 4000, NetSent: 110,
		MemTotal: 8, MemUsed: 2, Disks: []Disk{{Total: 100, Used: 40}, {Total: 50, Used: 5}}}
	s := Between(prev, cur)
	if s.CPU != 50 || s.DiskRead != 2000 || s.NetRecv != 0 || s.NetSent != 10 || s.DiskTotal != 150 || s.DiskUsed != 45 || s.MemUsed != 2 {
		t.Fatalf("%+v", s)
	}
}

func TestFilters(t *testing.T) {
	for name, want := range map[string]bool{"eth0": true, "en0": true, "wlan0": true, "enp3s0": true, "lo": false, "lo0": false, "veth12ab": false, "docker0": false, "br-1a2b": false, "utun3": false, "tailscale0": false} {
		if PhysicalInterface(name) != want {
			t.Errorf("interface %s", name)
		}
	}
	for name, want := range map[string]bool{"sda": true, "sda1": false, "nvme0n1": true, "nvme0n1p2": false, "vda": true, "loop0": false, "disk0": true, "dm-0": false} {
		if WholeDisk(name) != want {
			t.Errorf("disk %s", name)
		}
	}
	if !RealFS("/dev/sda1", "ext4", "/") || RealFS("tmpfs", "tmpfs", "/run") || RealFS("/dev/loop3", "squashfs", "/snap/x") ||
		RealFS("/dev/disk3s1s1", "apfs", "/System/Volumes/Preboot") || !RealFS("/dev/disk3s5", "apfs", "/System/Volumes/Data") {
		t.Error("RealFS")
	}
	got := DedupeDisks([]Disk{{Device: "/dev/sda1", Mount: "/srv"}, {Device: "/dev/sda1", Mount: "/"}, {Device: "/dev/sdb1"}})
	if len(got) != 2 || got[0].Mount != "/" || RealFS("/dev/vdb1", "btrfs", "/etc/hosts") {
		t.Errorf("dedupe %+v", got)
	}
}

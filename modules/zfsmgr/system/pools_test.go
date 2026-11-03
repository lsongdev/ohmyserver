package system

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func readStatus(t *testing.T, file string) Pool {
	t.Helper()
	data, err := os.ReadFile(filepath.Join("testdata", file))
	if err != nil {
		t.Fatal(err)
	}
	pools, err := parseStatus(data)
	if err != nil || len(pools) != 1 {
		t.Fatalf("%v %v", pools, err)
	}
	return pools["omspool"]
}

func TestStatusDegraded(t *testing.T) {
	p := readStatus(t, "zpool-degraded.json")
	if p.Health != "DEGRADED" || p.Scan != nil {
		t.Fatalf("%+v", p)
	}
	if p.Status != "One or more devices has been taken offline by the administrator. Sufficient replicas exist for the pool to continue functioning in a degraded state." {
		t.Fatalf("status %q", p.Status)
	}
	if len(p.Vdevs) != 4 {
		t.Fatalf("%+v", p.Vdevs)
	}
	mirror := p.Vdevs[0]
	if mirror.Type != "mirror" || mirror.State != "DEGRADED" || mirror.Children[1].State != "OFFLINE" {
		t.Fatalf("%+v", mirror)
	}
	// zpool's order, and the special devices after the data.
	if mirror.Children[0].Name != "/tmp/omspool/a.img" || p.Vdevs[1].Class != "log" || p.Vdevs[2].Class != "l2cache" {
		t.Fatalf("%+v", p.Vdevs)
	}
	if spare := p.Vdevs[3]; spare.Class != "spare" || spare.State != "AVAIL" {
		t.Fatalf("%+v", spare)
	}
}

func TestStatusScans(t *testing.T) {
	scrub := readStatus(t, "zpool-scrubbed.json").Scan
	if scrub.Function != "SCRUB" || scrub.State != "FINISHED" || scrub.End == 0 || scrub.Issued == 0 || scrub.Errors != 0 {
		t.Fatalf("%+v", scrub)
	}
	p := readStatus(t, "zpool-resilver.json")
	if p.Scan.Function != "RESILVER" || p.Scan.State != "SCANNING" || p.Scan.End != 0 || p.Scan.Repaired == 0 {
		t.Fatalf("%+v", p.Scan)
	}
	if replacing := p.Vdevs[0].Children[1]; replacing.Type != "replacing" || len(replacing.Children) != 2 {
		t.Fatalf("%+v", replacing)
	}
}

// zpoolFake answers zpool with fixed output, or never returns.
type zpoolFake struct {
	fake
	stuck bool
}

func (f *zpoolFake) Run(ctx context.Context, name string, args ...string) ([]byte, error) {
	if f.stuck {
		select {} // stuck in the kernel: not even the context ends it
	}
	return f.fake.Run(ctx, name, args...)
}

func TestAddHealthWhenThereIsNone(t *testing.T) {
	ctx := context.Background()
	pools := []Pool{{Name: "data", Health: "ONLINE"}}
	z := &ZFS{Run: &zpoolFake{fake: fake{out: map[string]string{"zpool status": "no pools available\n"}}}}
	if err := z.AddHealth(ctx, pools); err != nil || pools[0].Health != "ONLINE" {
		t.Fatalf("%+v %v", pools, err)
	}
	z = &ZFS{Run: &zpoolFake{fake: fake{fail: map[string]string{"zpool status -j --json-int": "invalid option 'j'"}}}}
	if err := z.AddHealth(ctx, pools); err == nil || err.Error() != "reading devices and scrubs needs OpenZFS 2.3 or later" {
		t.Fatalf("old zpool: %v", err)
	}
}

func TestPoolsDoNotHangOnAStuckZpool(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()
	if _, err := (&ZFS{Run: &zpoolFake{stuck: true}}).Pools(ctx); err == nil {
		t.Fatal("expected an error")
	}
}

func TestLinkDisks(t *testing.T) {
	root := t.TempDir()
	for _, dir := range []string{"dev/disk/by-id", "sys/class/block", "sys/devices/x/block/sda/sda1", "sys/devices/x/block/sdb"} {
		if err := os.MkdirAll(filepath.Join(root, dir), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	for _, name := range []string{"sda1", "sdb"} {
		if err := os.WriteFile(filepath.Join(root, "dev", name), nil, 0o644); err != nil {
			t.Fatal(err)
		}
	}
	os.Symlink("../../sda1", filepath.Join(root, "dev/disk/by-id/ata-A-part1"))
	os.Symlink("../../sdb", filepath.Join(root, "dev/disk/by-id/ata-B"))
	os.Symlink("../../devices/x/block/sda/sda1", filepath.Join(root, "sys/class/block/sda1"))
	os.Symlink("../../devices/x/block/sdb", filepath.Join(root, "sys/class/block/sdb"))
	vdevs := []Vdev{{Name: "raidz1-0", Type: "raidz", Children: []Vdev{
		{Name: "ata-A", Type: "disk", path: "/dev/disk/by-id/ata-A-part1"}, // a partition
		{Name: "ata-B", Type: "disk", path: "/dev/disk/by-id/ata-B"},       // a whole disk
		{Name: "ata-C", Type: "disk", path: "/dev/disk/by-id/ata-C-part1"}, // gone
		{Name: "/tmp/f", Type: "file", path: "/dev/sdb"},                   // not a disk
	}}}
	(&ZFS{Root: root}).linkDisks(vdevs)
	got := vdevs[0].Children
	if got[0].Disk != "sda" || got[1].Disk != "sdb" || got[2].Disk != "" || got[3].Disk != "" {
		t.Fatalf("%+v", got)
	}
}

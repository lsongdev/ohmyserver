package system

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"
)

func readPools(t *testing.T, file string) Pool {
	t.Helper()
	data, err := os.ReadFile(filepath.Join("testdata", file))
	if err != nil {
		t.Fatal(err)
	}
	pools, err := parsePools(data)
	if err != nil || len(pools) != 1 {
		t.Fatalf("%v %v", pools, err)
	}
	return pools[0]
}

func TestParsePoolsDegraded(t *testing.T) {
	p := readPools(t, "zpool-degraded.json")
	if p.Name != "omspool" || p.State != "DEGRADED" || p.Size == 0 || p.Scan != nil {
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

func TestParsePoolsScans(t *testing.T) {
	scrub := readPools(t, "zpool-scrubbed.json").Scan
	if scrub.Function != "SCRUB" || scrub.State != "FINISHED" || scrub.End == 0 || scrub.Issued == 0 || scrub.Errors != 0 {
		t.Fatalf("%+v", scrub)
	}
	p := readPools(t, "zpool-resilver.json")
	if p.Scan.Function != "RESILVER" || p.Scan.State != "SCANNING" || p.Scan.End != 0 || p.Scan.Repaired == 0 {
		t.Fatalf("%+v", p.Scan)
	}
	if replacing := p.Vdevs[0].Children[1]; replacing.Type != "replacing" || len(replacing.Children) != 2 {
		t.Fatalf("%+v", replacing)
	}
}

// poolRunner answers zpool with a fixed result, or never.
type poolRunner struct {
	Exec
	out   string
	err   error
	block bool
}

func (r poolRunner) Output(ctx context.Context, _ []string, _ string, _ ...string) ([]byte, error) {
	if r.block {
		select {} // stuck in the kernel: not even the context ends it
	}
	return []byte(r.out), r.err
}

func TestPoolsWithoutZpoolOrPools(t *testing.T) {
	ctx := context.Background()
	for name, r := range map[string]poolRunner{
		"not installed": {err: exec.ErrNotFound},
		"no pools":      {out: "no pools available\n"},
		"empty":         {out: `{"pools":{}}`},
	} {
		pools, err := (&Storage{Run: r}).Pools(ctx)
		if err != nil || len(pools) != 0 {
			t.Errorf("%s: %v %v", name, pools, err)
		}
	}
	_, err := (&Storage{Run: poolRunner{err: &ExitError{Command: "zpool", Code: 2, Output: "invalid option 'j'"}}}).Pools(ctx)
	if err == nil || err.Error() != "reading pool health needs OpenZFS 2.3 or later" {
		t.Fatalf("old zpool: %v", err)
	}
	_, err = (&Storage{Run: poolRunner{err: errors.New("broken")}}).Pools(ctx)
	if err == nil {
		t.Fatal("a failure must be reported")
	}
}

func TestPoolsDoNotHangOnAStuckZpool(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()
	if _, err := (&Storage{Run: poolRunner{block: true}}).Pools(ctx); err == nil {
		t.Fatal("expected an error")
	}
}

func TestLinkDisks(t *testing.T) {
	root := t.TempDir()
	byID := filepath.Join(root, "dev", "disk", "by-id")
	if err := os.MkdirAll(byID, 0o755); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"sda1", "sdb"} {
		if err := os.WriteFile(filepath.Join(root, "dev", name), nil, 0o644); err != nil {
			t.Fatal(err)
		}
	}
	os.Symlink("../../sda1", filepath.Join(byID, "ata-A-part1"))
	os.Symlink("../../sdb", filepath.Join(byID, "ata-B"))
	const doc = `{"pools":{"p":{"name":"p","state":"ONLINE","vdevs":{"p":{"name":"p","vdev_type":"root","vdevs":{
		"raidz1-0":{"name":"raidz1-0","vdev_type":"raidz","state":"ONLINE","vdevs":{
			"ata-A":{"name":"ata-A","vdev_type":"disk","state":"ONLINE","path":"/dev/disk/by-id/ata-A-part1"},
			"ata-B":{"name":"ata-B","vdev_type":"disk","state":"ONLINE","path":"/dev/disk/by-id/ata-B"},
			"ata-C":{"name":"ata-C","vdev_type":"disk","state":"FAULTED","path":"/dev/disk/by-id/ata-C-part1"}
		}}}}}}}}`
	pools, err := parsePools([]byte(doc))
	if err != nil {
		t.Fatal(err)
	}
	(&Storage{Root: root}).LinkDisks(pools, []Block{
		{Name: "sda", Children: []Block{{Name: "sda1"}}},
		{Name: "sdb"},
	})
	members := pools[0].Vdevs[0].Children
	if members[0].Disk != "sda" || members[1].Disk != "sdb" || members[2].Disk != "" {
		t.Fatalf("%+v", members)
	}
}

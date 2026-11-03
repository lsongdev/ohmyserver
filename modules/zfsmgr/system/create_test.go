package system

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

const lsblkDisks = `{"blockdevices":[
 {"name":"sda","type":"disk","size":1000000000,"model":"Pool Disk","rota":false,"children":[{"name":"sda1","type":"part","size":999,"fstype":"zfs_member","label":"data"}]},
 {"name":"sdb","type":"disk","size":"1000000000","model":"Free A ","serial":"A1","tran":"sata","rota":"0"},
 {"name":"sdc","type":"disk","size":2000000000,"model":"Free B","rota":true},
 {"name":"sdd","type":"disk","size":1000000000,"pttype":"gpt","children":[{"name":"sdd1","type":"part","size":999,"pttype":"gpt"},{"name":"sdd2","type":"part","size":999,"mountpoint":"/","pttype":"gpt"}]},
 {"name":"sdj","type":"disk","size":1000000000,"pttype":"gpt","children":[{"name":"sdj1","type":"part","size":999,"pttype":"gpt"}]},
 {"name":"sde","type":"disk","size":1000000000,"pttype":"gpt"},
 {"name":"sdf","type":"disk","size":1000000000,"fstype":"ext4"},
 {"name":"sdg","type":"disk","size":1000000000},
 {"name":"sdh","type":"disk","size":1000},
 {"name":"sdi","type":"disk","size":1000000000},
 {"name":"sr0","type":"rom","size":1000000000},
 {"name":"zram0","type":"disk","size":1000000000},
 {"name":"loop0","type":"loop","size":1000000000}
]}`

// sdg looks clean to lsblk, but ZFS says it is in a pool.
const zpoolWithSdg = `{"pools":{"tank":{"name":"tank","state":"ONLINE","vdevs":{"tank":{"name":"tank","vdev_type":"root","vdevs":{
 "ata-G":{"name":"ata-G","vdev_type":"disk","state":"ONLINE","path":"/dev/disk/by-id/ata-G"}}}}}}}`

// createSetup has stable names for every disk but sdi.
func createSetup(t *testing.T) (*ZFS, *fake) {
	t.Helper()
	root := t.TempDir()
	for _, dir := range []string{"dev/disk/by-id", "sys/class/block", "sys/devices/x/block/sdg"} {
		os.MkdirAll(filepath.Join(root, dir), 0o755)
	}
	for _, name := range []string{"sda", "sdb", "sdc", "sdd", "sde", "sdf", "sdg", "sdh"} {
		os.WriteFile(filepath.Join(root, "dev", name), nil, 0o644)
		id := "ata-" + strings.ToUpper(name[2:])
		os.Symlink("../../"+name, filepath.Join(root, "dev/disk/by-id", id))
		os.Symlink("../../"+name, filepath.Join(root, "dev/disk/by-id", "wwn-"+name))
		os.Symlink("../../"+name+"1", filepath.Join(root, "dev/disk/by-id", id+"-part1"))
	}
	os.Symlink("../../devices/x/block/sdg", filepath.Join(root, "sys/class/block/sdg"))
	f := &fake{out: map[string]string{
		"lsblk":        lsblkDisks,
		"zpool list":   "tank\t100\t1\t99\t1\tONLINE\n",
		"zpool status": zpoolWithSdg,
	}}
	return &ZFS{Run: f, Root: root}, f
}

func created(f *fake) []string {
	var calls []string
	for _, c := range f.calls {
		if strings.HasPrefix(c, "zpool create") {
			calls = append(calls, c)
		}
	}
	return calls
}

func TestDisksSayWhyNotFree(t *testing.T) {
	z, _ := createSetup(t)
	disks, err := z.Disks(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	want := map[string]string{
		"sda": "belongs to ZFS pool data", "sdb": "", "sdc": "", "sdd": "mounted at /", "sde": "has a partition table",
		"sdf": "holds a ext4 file system", "sdg": "belongs to ZFS pool tank", "sdh": "too small for a pool", "sdi": "has no stable device name", "sdj": "has partitions",
	}
	if len(disks) != len(want) {
		t.Fatalf("%+v", disks)
	}
	for _, d := range disks {
		if use, ok := want[d.Name]; !ok || d.InUse != use {
			t.Errorf("%s: %q, want %q", d.Name, d.InUse, use)
		}
	}
	if d := disks[1]; d.ID != "/dev/disk/by-id/ata-B" || d.Model != "Free A" || d.Size != 1000000000 || d.Rotational {
		t.Fatalf("%+v", d)
	}
	if !disks[2].Rotational {
		t.Fatalf("%+v", disks[2])
	}
}

func TestDisksFailClosedWithoutPoolMembers(t *testing.T) {
	z, f := createSetup(t)
	f.fail = map[string]string{"zpool status -j --json-int": "broken"}
	if _, err := z.Disks(context.Background()); err == nil {
		t.Fatal("offered disks without knowing what the pools hold")
	}
}

func TestLoopModeOffersNoRealDisk(t *testing.T) {
	z, _ := createSetup(t)
	z.TestLoopDisks = true
	disks, err := z.Disks(context.Background())
	if err != nil || len(disks) != 1 || disks[0].ID != "/dev/loop0" || disks[0].InUse != "" {
		t.Fatalf("%+v %v", disks, err)
	}
	if err := z.CreatePool(context.Background(), NewPool{Name: "t", Layout: "stripe", Disks: []string{"/dev/disk/by-id/ata-B"}}); err == nil {
		t.Fatal("accepted a real disk")
	}
}

func TestCreatePool(t *testing.T) {
	ctx := context.Background()
	const b, c = "/dev/disk/by-id/ata-B", "/dev/disk/by-id/ata-C"
	z, f := createSetup(t)
	if err := z.CreatePool(ctx, NewPool{Name: "tank2", Layout: "mirror", Disks: []string{b, c}, Mountpoint: "/srv/t"}); err != nil {
		t.Fatal(err)
	}
	preview, err := z.PreviewPool(ctx, NewPool{Name: "tank2", Layout: "stripe", Disks: []string{b}})
	if err != nil {
		t.Fatal(err)
	}
	want := []string{
		"zpool create -o ashift=12 -m /srv/t tank2 mirror " + b + " " + c,
		"zpool create -n -o ashift=12 tank2 " + b,
	}
	if got := created(f); !slices.Equal(got, want) {
		t.Fatalf("ran %q", got)
	}
	if preview.Usable != 1000000000 || len(preview.Warnings) != 1 || !strings.Contains(preview.Warnings[0], "No redundancy") {
		t.Fatalf("%+v", preview)
	}
	preview, err = z.PreviewPool(ctx, NewPool{Name: "tank2", Layout: "mirror", Disks: []string{b, c}})
	if err != nil || preview.Usable != 1000000000 || len(preview.Warnings) != 1 || !strings.Contains(preview.Warnings[0], "differ") {
		t.Fatalf("%+v %v", preview, err)
	}
	for _, c := range f.calls {
		if strings.Contains(c, " -f") {
			t.Fatalf("forced: %s", c)
		}
	}
}

func TestCreatePoolRefuses(t *testing.T) {
	ctx := context.Background()
	const a, b, c = "/dev/disk/by-id/ata-A", "/dev/disk/by-id/ata-B", "/dev/disk/by-id/ata-C"
	z, f := createSetup(t)
	os.MkdirAll(filepath.Join(z.Root, "busy"), 0o755)
	os.WriteFile(filepath.Join(z.Root, "busy/file"), nil, 0o644)
	for name, bad := range map[string]NewPool{
		"disk in a pool":      {Name: "t", Layout: "stripe", Disks: []string{a}},
		"one bad among good":  {Name: "t", Layout: "mirror", Disks: []string{b, a}},
		"disk ZFS knows":      {Name: "t", Layout: "stripe", Disks: []string{"/dev/disk/by-id/ata-G"}},
		"unknown disk":        {Name: "t", Layout: "stripe", Disks: []string{"/dev/sdb"}},
		"option as disk":      {Name: "t", Layout: "stripe", Disks: []string{"-f"}},
		"empty disk":          {Name: "t", Layout: "stripe", Disks: []string{""}},
		"twice":               {Name: "t", Layout: "mirror", Disks: []string{b, b}},
		"too few for mirror":  {Name: "t", Layout: "mirror", Disks: []string{b}},
		"too few for raidz1":  {Name: "t", Layout: "raidz1", Disks: []string{b, c}},
		"no disks":            {Name: "t", Layout: "stripe"},
		"unknown layout":      {Name: "t", Layout: "-f", Disks: []string{b}},
		"reserved name":       {Name: "mirror1", Layout: "stripe", Disks: []string{b}},
		"option as name":      {Name: "-f", Layout: "stripe", Disks: []string{b}},
		"name with slash":     {Name: "a/b", Layout: "stripe", Disks: []string{b}},
		"relative mount":      {Name: "t", Layout: "stripe", Disks: []string{b}, Mountpoint: "srv"},
		"root mount":          {Name: "t", Layout: "stripe", Disks: []string{b}, Mountpoint: "/"},
		"mount that has data": {Name: "t", Layout: "stripe", Disks: []string{b}, Mountpoint: "/busy"},
	} {
		if err := z.CreatePool(ctx, bad); err == nil {
			t.Errorf("%s: accepted", name)
		}
		if _, err := z.PreviewPool(ctx, bad); err == nil {
			t.Errorf("%s: previewed", name)
		}
	}
	if got := created(f); got != nil {
		t.Fatalf("ran %q", got)
	}
	err := z.CreatePool(ctx, NewPool{Name: "t", Layout: "stripe", Disks: []string{a}})
	if !errors.Is(err, ErrConflict) || !strings.Contains(err.Error(), "sda belongs to ZFS pool data") {
		t.Fatalf("%v", err)
	}
}

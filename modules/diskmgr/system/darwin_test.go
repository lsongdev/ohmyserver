package system

import (
	"context"
	"os"
	"strings"
	"testing"
)

// fixtures replays diskutil, df and mount output recorded on a MacBook
// (Apple Silicon, macOS 27) with Xcode simulator disk images mounted.
type fixtures struct{ t *testing.T }

func (f fixtures) Output(_ context.Context, _ []string, name string, args ...string) ([]byte, error) {
	file := map[string]string{
		"diskutil list -plist":          "darwin-list.plist",
		"diskutil list -plist physical": "darwin-list-physical.plist",
		"diskutil info -plist disk0":    "darwin-info-disk0.plist",
		"df -k -P":                      "darwin-df.txt",
		"mount":                         "darwin-mount.txt",
	}[strings.Join(append([]string{name}, args...), " ")]
	if file == "" {
		f.t.Fatalf("unexpected command %s %v", name, args)
	}
	return os.ReadFile("testdata/" + file)
}

func (f fixtures) Stream(context.Context, []string, func(string), string, ...string) error { return nil }

func TestDarwinBlocks(t *testing.T) {
	s := &Storage{Run: fixtures{t}, OS: "darwin"}
	blocks, err := s.Blocks(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	disks := Disks(blocks)
	// Only the internal SSD: simulator disk images are not physical.
	if len(disks) != 1 {
		t.Fatalf("disks = %+v", disks)
	}
	d := disks[0]
	if d.Name != "disk0" || d.Model != "APPLE SSD AP0256Z" || d.Rotational || d.Transport != "nvme" || d.Size != 251000193024 || len(d.Children) != 3 {
		t.Fatalf("disk0 = %+v", d)
	}
	apfs := d.Children[1]
	if apfs.Name != "disk0s2" || apfs.FSType != "apfs" || len(apfs.Children) != 1 || apfs.Children[0].Name != "disk3" {
		t.Fatalf("disk0s2 = %+v", apfs)
	}
	var root *Block
	for i, v := range apfs.Children[0].Children {
		if strings.Contains(v.Name, "s1s1") {
			t.Errorf("snapshot %s listed as a volume", v.Name)
		}
		if v.Mountpoint == "/" {
			root = &apfs.Children[0].Children[i]
		}
	}
	if root == nil || root.Name != "disk3s1" || root.Label != "Macintosh HD" {
		t.Fatalf("root volume = %+v", root)
	}
	var mounts []string
	var collect func(b Block)
	collect = func(b Block) {
		if b.Mountpoint != "" {
			mounts = append(mounts, b.Mountpoint)
		}
		for _, c := range b.Children {
			collect(c)
		}
	}
	collect(d)
	// Recovery is a volume people mount on purpose; the rest is macOS's own.
	if strings.Join(mounts, " ") != "/ /Volumes/Recovery" {
		t.Fatalf("mount points = %v", mounts)
	}
}

func TestDarwinFileSystems(t *testing.T) {
	s := &Storage{Run: fixtures{t}, OS: "darwin"}
	list, err := s.FileSystems(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	// The startup disk's container once, at /, with the container's use;
	// hidden system and simulator volumes are left out.
	if len(list) != 1 {
		t.Fatalf("file systems = %+v", list)
	}
	fs := list[0]
	if fs.Source != "/dev/disk3" || fs.Mountpoint != "/" || fs.Type != "apfs" || fs.Size != 239331776*1024 ||
		fs.Available != 25344056*1024 || fs.Used != fs.Size-fs.Available {
		t.Fatalf("root = %+v", fs)
	}
}

func TestDarwinExternalVolumeAndSpaces(t *testing.T) {
	df := []byte("Filesystem 1024-blocks Used Available Capacity Mounted on\n" +
		"/dev/disk3s1s1 1000 600 400 60% /\n" +
		"/dev/disk12s1 2000 500 1500 25% /Volumes/My Passport\n" +
		"//user@nas/share 3000 1000 2000 34% /Volumes/share\n")
	mount := []byte("/dev/disk3s1s1 on / (apfs, sealed, local, read-only, journaled)\n" +
		"/dev/disk12s1 on /Volumes/My Passport (exfat, local, nodev, nosuid, noowners)\n" +
		"//user@nas/share on /Volumes/share (smbfs, nodev, nosuid, mounted by lsong)\n")
	list := parseDarwinFileSystems(df, mount)
	if len(list) != 3 || list[1].Mountpoint != "/Volumes/My Passport" || list[1].Type != "exfat" || list[1].Used != 500*1024 ||
		list[2].Source != "//user@nas/share" || list[2].Type != "smbfs" {
		t.Fatalf("file systems = %+v", list)
	}
}

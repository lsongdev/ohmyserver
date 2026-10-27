package system

import (
	"fmt"
	"strings"
	"testing"

	"github.com/lsongdev/smartctl-go/smartctl"
)

// util-linux 2.32 (Alibaba Cloud Linux 3) prints everything as strings.
const lsblk232 = `{
   "blockdevices": [
      {"name": "vda", "type": "disk", "size": "42949672960", "model": null, "serial": "2zehiqznzhix0qm3lfvk", "rota": "1", "tran": null, "rm": "0", "mountpoint": null, "fstype": null, "label": null,
         "children": [
            {"name": "vda3", "type": "part", "size": "42736795136", "model": null, "serial": null, "rota": "1", "tran": null, "rm": "0", "mountpoint": "/", "fstype": "ext4", "label": "root"}
         ]
      },
      {"name": "sr0", "type": "rom", "size": "1073741312", "model": "QEMU DVD-ROM", "serial": null, "rota": "1", "tran": "ata", "rm": "1", "mountpoint": null, "fstype": null, "label": null}
   ]
}`

// util-linux 2.39 (Ubuntu) prints JSON values.
const lsblk239 = `{"blockdevices": [
  {"name": "sda", "type": "disk", "size": 1024209543168, "model": "Samsung SSD 850", "serial": "S226NX0H500689R", "rota": false, "tran": "sata", "rm": false, "mountpoint": null, "fstype": "linux_raid_member", "label": "lsong-nas:0",
   "children": [{"name": "md0", "type": "raid5", "size": 5120369623040, "model": null, "serial": null, "rota": false, "tran": null, "rm": false, "mountpoint": "/data", "fstype": "btrfs", "label": null}]},
  {"name": "sdg", "type": "disk", "size": 15664676864, "model": "Cruzer Fit", "serial": "4C531001591018110021", "rota": true, "tran": "usb", "rm": true, "mountpoint": null, "fstype": null, "label": null},
  {"name": "zram0", "type": "disk", "size": 8589934592, "rota": false, "rm": false}
]}`

func TestLsblkBothFormats(t *testing.T) {
	old, err := parseLsblk([]byte(lsblk232))
	if err != nil {
		t.Fatal(err)
	}
	if d := old[0]; d.Size != 42949672960 || !d.Rotational || d.Removable || d.Children[0].Mountpoint != "/" || d.Children[0].Label != "root" {
		t.Fatalf("2.32: %+v", d)
	}
	recent, err := parseLsblk([]byte(lsblk239))
	if err != nil {
		t.Fatal(err)
	}
	if d := recent[1]; d.Size != 15664676864 || !d.Rotational || !d.Removable || d.Transport != "usb" || d.Model != "Cruzer Fit" {
		t.Fatalf("2.39: %+v", d)
	}
	var names []string
	for _, d := range Disks(append(old, recent...)) {
		names = append(names, d.Name)
	}
	if strings.Join(names, " ") != "vda sda sdg" {
		t.Fatalf("disks %v", names)
	}
}

func TestDf(t *testing.T) {
	out := `Filesystem     Type      1B-blocks          Used        Avail Mounted on
/dev/sdg2      ext4    15306510336    8054079488   6452580352 /
/dev/md0       btrfs 5120369623040 4261816414208 856204070912 /data/photos
/dev/md0       btrfs 5120369623040 4261816414208 856204070912 /data
nas:/export    nfs4   0 0 0 /mnt/broken
/dev/sdh1      exfat  64000000000  1000 63999999000 /media/My Disk
`
	got := fmt.Sprint(parseDf([]byte(out)))
	want := "[{/dev/sdg2 ext4 15306510336 8054079488 6452580352 /} {/dev/md0 btrfs 5120369623040 4261816414208 856204070912 /data} {/dev/sdh1 exfat 64000000000 1000 63999999000 /media/My Disk}]"
	if got != want {
		t.Fatalf("got  %s\nwant %s", got, want)
	}
}

func TestMdstat(t *testing.T) {
	// The NAS, then a degraded mirror rebuilding and an inactive array.
	data := `Personalities : [raid6] [raid5] [raid4] [linear] [multipath] [raid0] [raid1] [raid10] 
md0 : active raid5 sdf[6] sde[4] sdd[3] sdc[2] sdb[1] sda[0]
      5000360960 blocks super 1.2 level 5, 512k chunk, algorithm 2 [6/6] [UUUUUU]
      bitmap: 0/8 pages [0KB], 65536KB chunk

md1 : active raid1 sdb1[2] sda1[0] sdc1[3](F) sdd1[4](S)
      976630464 blocks super 1.2 [2/1] [U_]
      [==>..................]  recovery = 12.6% (123456/976630464) finish=95.3min speed=150000K/sec

md2 : inactive sdx[0](S)
      976630464 blocks super 1.2

unused devices: <none>
`
	arrays := parseMdstat(data)
	if len(arrays) != 3 {
		t.Fatalf("arrays %+v", arrays)
	}
	if a := arrays[0]; a.Level != "raid5" || a.Size != 5000360960*1024 || len(a.Members) != 6 || a.Degraded || a.Disks != 6 || a.Working != 6 || a.Sync != nil {
		t.Fatalf("md0 %+v", a)
	}
	a := arrays[1]
	if !a.Degraded || a.Working != 1 || !a.Members[2].Faulty || !a.Members[3].Spare || a.Sync == nil ||
		a.Sync.Action != "recovery" || a.Sync.Progress != 12.6 || a.Sync.Finish != "95.3min" || a.Sync.Speed != "150000K/sec" {
		t.Fatalf("md1 %+v %+v", a, a.Sync)
	}
	if a := arrays[2]; a.State != "inactive" || a.Level != "" || len(a.Members) != 1 {
		t.Fatalf("md2 %+v", a)
	}
}

func TestSummarize(t *testing.T) {
	info, err := smartctl.Open("testdata/samsung-850-pro.json")
	if err != nil {
		t.Fatal(err)
	}
	h := Summarize(info)
	if h.Passed == nil || !*h.Passed || h.Temperature != 30 || h.PowerOnHours != 98678 || h.Wear == nil || *h.Wear != 9 || len(h.Warnings) != 0 {
		t.Fatalf("samsung %+v", h)
	}
	info, _ = smartctl.Open("testdata/n600s.json")
	if h := Summarize(info); h.Temperature != 45 || h.PowerOnHours != 23268 || len(h.Warnings) != 0 {
		t.Fatalf("n600s %+v", h)
	}
	// The same drive, worn and with errors.
	for i, a := range info.ATASmartAttributes.Table {
		switch a.ID {
		case 5:
			info.ATASmartAttributes.Table[i].Raw.Value = 8
		case 199:
			info.ATASmartAttributes.Table[i].Raw.Value = 3
		}
	}
	info.SmartStatus.Passed = false
	info.Temperature.Current = 63
	h = Summarize(info)
	want := []string{"failing", "8 reallocated sectors", "3 interface CRC errors", "Running hot at 63"}
	for _, w := range want {
		if !strings.Contains(strings.Join(h.Warnings, "\n"), w) {
			t.Errorf("missing %q in %q", w, h.Warnings)
		}
	}
}

func TestDiskName(t *testing.T) {
	for _, ok := range []string{"sda", "nvme0n1", "mmcblk0", "vdb"} {
		if _, err := DiskName(ok); err != nil {
			t.Errorf("%s: %v", ok, err)
		}
	}
	for _, bad := range []string{"", "../sda", "sda;", "-sda", "SDA", "s"} {
		if _, err := DiskName(bad); err == nil {
			t.Errorf("%q accepted", bad)
		}
	}
}

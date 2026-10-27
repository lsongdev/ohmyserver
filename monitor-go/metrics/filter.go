package metrics

import (
	"regexp"
	"strings"
)

// Totals count physical devices only: virtual interfaces and partitions
// would count the same bytes twice.

var virtualInterface = regexp.MustCompile(`^(lo\d*|veth|docker|br-|virbr|cni|flannel|kube|cali|tun|tap|utun|gif|stf|awdl|llw|anpi|bridge|ap\d|vmnet|vboxnet|zt|tailscale|wg)`)

// PhysicalInterface reports whether a network interface counts towards
// the host's traffic.
func PhysicalInterface(name string) bool { return !virtualInterface.MatchString(name) }

var wholeDisk = regexp.MustCompile(`^(sd[a-z]+|hd[a-z]+|vd[a-z]+|xvd[a-z]+|nvme\d+n\d+|mmcblk\d+|disk\d+)$`)

// WholeDisk reports whether a block device is a whole disk rather than a
// partition, loop or RAM device.
func WholeDisk(name string) bool { return wholeDisk.MatchString(name) }

var pseudoFS = map[string]bool{
	"tmpfs": true, "devtmpfs": true, "overlay": true, "squashfs": true, "autofs": true, "devfs": true,
	"proc": true, "sysfs": true, "cgroup": true, "cgroup2": true, "nsfs": true, "fuse.lxcfs": true,
}

// RealFS reports whether a mounted file system is storage worth reporting:
// backed by a device, not a pseudo file system, a loop image or a read-only
// system snapshot.
func RealFS(device, fstype, mount string) bool {
	switch {
	case pseudoFS[fstype]:
		return false
	case !strings.HasPrefix(device, "/dev/") || strings.HasPrefix(device, "/dev/loop"):
		return false
	case strings.HasPrefix(mount, "/System/Volumes/") && mount != "/System/Volumes/Data",
		mount == "/Volumes/Recovery", strings.HasPrefix(mount, "/Library/Developer/"):
		return false // macOS system, recovery and simulator volumes
	case strings.HasPrefix(mount, "/snap/"), strings.HasPrefix(mount, "/boot/efi"):
		return false
	case strings.HasPrefix(mount, "/etc/"):
		return false // files bind-mounted into containers (/etc/hosts, ...)
	}
	return true
}

// DedupeDisks keeps one entry per device, the one with the shortest mount
// point: bind mounts and btrfs subvolumes show the same space several
// times. The order of first appearance is kept.
func DedupeDisks(disks []Disk) []Disk {
	index := map[string]int{}
	var out []Disk
	for _, d := range disks {
		i, ok := index[d.Device]
		switch {
		case !ok:
			index[d.Device] = len(out)
			out = append(out, d)
		case len(d.Mount) < len(out[i].Mount):
			out[i] = d
		}
	}
	return out
}

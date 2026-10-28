// Package system reads a host's storage: block devices (lsblk on Linux,
// diskutil on macOS), mounted file systems and their capacity (df),
// software RAID arrays (/proc/mdstat, Linux) and disk health (smartctl,
// through smartctl-go).
//
// Commands run with LC_ALL=C and never through a shell. Output formats are
// those of util-linux 2.32 (Enterprise Linux 8) and later, whose lsblk
// prints numbers and flags as strings before 2.33 and as JSON values after.
package system

import (
	"context"
	"encoding/json"
	"errors"
	"os/exec"
	"regexp"
	"runtime"
	"strconv"
	"strings"
)

// ErrUnsupported means the host has neither lsblk (Linux) nor diskutil
// (macOS).
var ErrUnsupported = errors.New("disk information needs Linux (lsblk) or macOS (diskutil)")

// ErrInvalid wraps rejected input, such as an unknown disk name.
var ErrInvalid = errors.New("invalid")

// Block is a block device: a disk, a partition, a RAID array, …
type Block struct {
	Name       string  `json:"name"` // sda, nvme0n1p1, md0
	Type       string  `json:"type"` // disk, part, raid5, lvm, crypt, rom, loop
	Size       int64   `json:"size"` // bytes
	Model      string  `json:"model,omitempty"`
	Serial     string  `json:"serial,omitempty"`
	Rotational bool    `json:"rotational"`          // a spinning disk
	Transport  string  `json:"transport,omitempty"` // sata, nvme, usb, …
	Removable  bool    `json:"removable,omitempty"`
	Mountpoint string  `json:"mountpoint,omitempty"`
	FSType     string  `json:"fstype,omitempty"`
	Label      string  `json:"label,omitempty"`
	Children   []Block `json:"children,omitempty"`
}

// flex reads a JSON string, number, boolean or null as text.
type flex string

func (f *flex) UnmarshalJSON(data []byte) error {
	var s string
	if json.Unmarshal(data, &s) == nil {
		*f = flex(s)
		return nil
	}
	if string(data) == "null" {
		*f = ""
		return nil
	}
	*f = flex(strings.Trim(string(data), `"`))
	return nil
}

func (f flex) bool() bool { return f == "true" || f == "1" }
func (f flex) int() int64 {
	n, _ := strconv.ParseInt(string(f), 10, 64)
	return n
}

type rawBlock struct {
	Name       flex       `json:"name"`
	Type       flex       `json:"type"`
	Size       flex       `json:"size"`
	Model      flex       `json:"model"`
	Serial     flex       `json:"serial"`
	Rota       flex       `json:"rota"`
	Tran       flex       `json:"tran"`
	RM         flex       `json:"rm"`
	Mountpoint flex       `json:"mountpoint"`
	FSType     flex       `json:"fstype"`
	Label      flex       `json:"label"`
	Children   []rawBlock `json:"children"`
}

func (r rawBlock) block() Block {
	b := Block{
		Name: string(r.Name), Type: string(r.Type), Size: r.Size.int(),
		Model: strings.TrimSpace(string(r.Model)), Serial: strings.TrimSpace(string(r.Serial)),
		Rotational: r.Rota.bool(), Transport: string(r.Tran), Removable: r.RM.bool(),
		Mountpoint: string(r.Mountpoint), FSType: string(r.FSType), Label: string(r.Label),
	}
	for _, c := range r.Children {
		b.Children = append(b.Children, c.block())
	}
	return b
}

const lsblkColumns = "NAME,TYPE,SIZE,MODEL,SERIAL,ROTA,TRAN,RM,MOUNTPOINT,FSTYPE,LABEL"

// Blocks lists the block devices as a tree: disks with their partitions
// and what is built on them. RAID arrays appear under each member disk.
func (s *Storage) Blocks(ctx context.Context) ([]Block, error) {
	if s.os() == "darwin" {
		return s.blocksDarwin(ctx)
	}
	out, err := s.Run.Output(ctx, nil, "lsblk", "-b", "-J", "-o", lsblkColumns)
	if err != nil {
		return nil, err
	}
	return parseLsblk(out)
}

func parseLsblk(out []byte) ([]Block, error) {
	var doc struct {
		Blockdevices []rawBlock `json:"blockdevices"`
	}
	if err := json.Unmarshal(out, &doc); err != nil {
		return nil, err
	}
	var result []Block
	for _, r := range doc.Blockdevices {
		result = append(result, r.block())
	}
	return result, nil
}

// Disks are the physical disks among blocks: not loops, RAM disks, optical
// drives or arrays.
func Disks(blocks []Block) []Block {
	var result []Block
	for _, b := range blocks {
		if b.Type == "disk" && !strings.HasPrefix(b.Name, "zram") && !strings.HasPrefix(b.Name, "ram") {
			result = append(result, b)
		}
	}
	return result
}

// A disk's kernel name: sda, vdb, nvme0n1, mmcblk0, xvda, …
var validDisk = regexp.MustCompile(`^[a-z][a-z0-9]{1,30}$`)

// DiskName validates a disk name taken from a request.
func DiskName(name string) (string, error) {
	if !validDisk.MatchString(name) {
		return "", errors.Join(ErrInvalid, errors.New("disk name "+strconv.Quote(name)))
	}
	return name, nil
}

// Storage reads the host's storage.
type Storage struct {
	Run  Runner
	Root string // file system root, "" on a real host
	OS   string // runtime.GOOS unless testing
}

func (s *Storage) os() string {
	if s.OS != "" {
		return s.OS
	}
	return runtime.GOOS
}

// Supported reports whether the host has the tools this package reads:
// lsblk on Linux, diskutil on macOS.
func (s *Storage) Supported() bool {
	tool := "lsblk"
	if s.os() == "darwin" {
		tool = "diskutil"
	}
	_, err := exec.LookPath(tool)
	return err == nil
}

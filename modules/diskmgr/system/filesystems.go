package system

import (
	"context"
	"strconv"
	"strings"
)

// FileSystem is a mounted file system and its capacity.
type FileSystem struct {
	Source     string `json:"source"` // /dev/md0, server:/export, …
	Type       string `json:"type"`
	Size       int64  `json:"size"`
	Used       int64  `json:"used"`
	Available  int64  `json:"available"`
	Mountpoint string `json:"mountpoint"`
}

// virtual file systems hold no data worth a capacity bar.
var virtual = []string{"tmpfs", "devtmpfs", "overlay", "squashfs", "efivarfs", "ramfs", "nsfs", "tracefs", "fuse.lxcfs", "fuse.snapfuse"}

// FileSystems lists mounted file systems with storage behind them, once
// per source (a btrfs volume mounted at several places shows once, at its
// shortest mount point).
func (s *Storage) FileSystems(ctx context.Context) ([]FileSystem, error) {
	args := []string{"-B1", "--output=source,fstype,size,used,avail,target"}
	for _, t := range virtual {
		args = append(args, "-x", t)
	}
	// df exits 1 when some mount cannot be read (a stale network mount)
	// while still printing the rest.
	out, err := s.Run.Output(ctx, nil, "df", args...)
	if err != nil && len(out) == 0 {
		return nil, err
	}
	return parseDf(out), nil
}

func parseDf(out []byte) []FileSystem {
	seen := map[string]int{}
	var result []FileSystem
	for i, line := range lines(out) {
		f := strings.Fields(line)
		if i == 0 || len(f) < 6 {
			continue // the header
		}
		number := func(s string) int64 { n, _ := strconv.ParseInt(s, 10, 64); return n }
		fs := FileSystem{Source: f[0], Type: f[1], Size: number(f[2]), Used: number(f[3]), Available: number(f[4]),
			Mountpoint: strings.Join(f[5:], " ")}
		if fs.Size == 0 {
			continue
		}
		if j, ok := seen[fs.Source]; ok {
			if len(fs.Mountpoint) < len(result[j].Mountpoint) {
				result[j] = fs
			}
			continue
		}
		seen[fs.Source] = len(result)
		result = append(result, fs)
	}
	return result
}

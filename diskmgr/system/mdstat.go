package system

import (
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
)

// Array is a Linux software RAID array (md).
type Array struct {
	Name    string   `json:"name"`  // md0
	State   string   `json:"state"` // active, inactive
	Level   string   `json:"level"` // raid1, raid5, …
	Size    int64    `json:"size"`  // bytes
	Members []Member `json:"members"`
	// Disks and Working count the devices the array should have and has
	// working ("[6/5]"); Degraded means fewer than it should.
	Disks    int  `json:"disks"`
	Working  int  `json:"working"`
	Degraded bool `json:"degraded"`
	ReadOnly bool `json:"read_only,omitempty"`
	// Sync is a resync, recovery, reshape or check in progress.
	Sync *Sync `json:"sync,omitempty"`
}

// Member is a device in an array.
type Member struct {
	Name   string `json:"name"`
	Role   int    `json:"role"`
	Faulty bool   `json:"faulty,omitempty"`
	Spare  bool   `json:"spare,omitempty"`
}

// Sync is an operation running over an array.
type Sync struct {
	Action   string  `json:"action"`   // recovery, resync, reshape, check
	Progress float64 `json:"progress"` // percent
	Finish   string  `json:"finish,omitempty"`
	Speed    string  `json:"speed,omitempty"`
}

// Arrays reads /proc/mdstat; a host without md arrays has none.
func (s *Storage) Arrays() []Array {
	data, err := os.ReadFile(filepath.Join(s.Root, "/proc/mdstat"))
	if err != nil {
		return nil
	}
	return parseMdstat(string(data))
}

var (
	memberRe = regexp.MustCompile(`^([^\[]+)\[(\d+)\](\((F|S|W|R)\))?$`)
	countRe  = regexp.MustCompile(`\[(\d+)/(\d+)\]`)
	syncRe   = regexp.MustCompile(`(recovery|resync|reshape|check|repair)\s*=\s*([\d.]+)%(?:.*finish=(\S+))?(?:.*speed=(\S+))?`)
	delayRe  = regexp.MustCompile(`(resync|recovery)=(DELAYED|PENDING)`)
)

func parseMdstat(data string) []Array {
	var result []Array
	var current *Array
	for _, line := range strings.Split(data, "\n") {
		if name, rest, ok := strings.Cut(line, " : "); ok && strings.HasPrefix(name, "md") {
			f := strings.Fields(rest)
			a := Array{Name: strings.TrimSpace(name)}
			if len(f) > 0 {
				a.State, f = f[0], f[1:]
			}
			if len(f) > 0 && strings.HasPrefix(f[0], "(") { // (read-only), (auto-read-only)
				a.ReadOnly, f = true, f[1:]
			}
			if len(f) > 0 && !strings.Contains(f[0], "[") {
				a.Level, f = f[0], f[1:]
			}
			for _, m := range f {
				if g := memberRe.FindStringSubmatch(m); g != nil {
					role, _ := strconv.Atoi(g[2])
					a.Members = append(a.Members, Member{Name: g[1], Role: role, Faulty: g[4] == "F", Spare: g[4] == "S"})
				}
			}
			result = append(result, a)
			current = &result[len(result)-1]
			continue
		}
		if current == nil {
			continue
		}
		if strings.TrimSpace(line) == "" {
			current = nil
			continue
		}
		if strings.Contains(line, " blocks") {
			f := strings.Fields(line)
			if len(f) > 0 {
				if n, err := strconv.ParseInt(f[0], 10, 64); err == nil {
					current.Size = n * 1024
				}
			}
			if g := countRe.FindStringSubmatch(line); g != nil {
				current.Disks, _ = strconv.Atoi(g[1])
				current.Working, _ = strconv.Atoi(g[2])
				current.Degraded = current.Working < current.Disks
			}
		}
		if g := syncRe.FindStringSubmatch(line); g != nil {
			progress, _ := strconv.ParseFloat(g[2], 64)
			current.Sync = &Sync{Action: g[1], Progress: progress, Finish: g[3], Speed: g[4]}
		} else if g := delayRe.FindStringSubmatch(line); g != nil {
			current.Sync = &Sync{Action: g[1] + " " + strings.ToLower(g[2])}
		}
	}
	return result
}

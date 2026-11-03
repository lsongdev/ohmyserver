package system

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"time"
)

// poolTimeout bounds asking zpool: on a suspended pool it can hang, and that
// must not take the rest of the page with it.
const poolTimeout = 5 * time.Second

// Pool is a ZFS storage pool's health, as zpool status reports it.
type Pool struct {
	Name   string `json:"name"`
	State  string `json:"state"`            // ONLINE, DEGRADED, FAULTED, …
	Status string `json:"status,omitempty"` // what is wrong, in zpool's words
	Action string `json:"action,omitempty"` // what to do about it
	Errors int64  `json:"errors"`           // files with known data errors
	Size   int64  `json:"size"`
	Alloc  int64  `json:"alloc"`
	Scan   *Scan  `json:"scan,omitempty"` // the latest scrub or resilver
	Vdevs  []Vdev `json:"vdevs"`          // in zpool's order: data, then log, cache, spare
}

// Scan is a pool's latest scrub or resilver; zpool keeps only that one.
type Scan struct {
	Function  string `json:"function"` // SCRUB, RESILVER
	State     string `json:"state"`    // SCANNING, FINISHED, CANCELED
	Start     int64  `json:"start"`    // unix seconds
	End       int64  `json:"end"`      // 0 while scanning
	ToExamine int64  `json:"to_examine"`
	Issued    int64  `json:"issued"`   // read and verified so far
	Repaired  int64  `json:"repaired"` // repaired or resilvered
	Errors    int64  `json:"errors"`   // what it could not repair
	Paused    bool   `json:"paused"`
}

// Vdev is a device in a pool, or a group of them.
type Vdev struct {
	Name     string `json:"name"`
	Type     string `json:"type"`  // raidz, mirror, disk, file, replacing, …
	Class    string `json:"class"` // normal, log, l2cache, spare, special, …
	State    string `json:"state"`
	Read     int64  `json:"read_errors"`
	Write    int64  `json:"write_errors"`
	Checksum int64  `json:"checksum_errors"`
	// Disk names the disk it is on, for vdevs that are whole disks or
	// partitions and can be found now.
	Disk     string `json:"disk,omitempty"`
	path     string
	Children []Vdev `json:"children,omitempty"`
}

// Pools asks zpool for the health of the pools. Without zpool, or without any
// pool, there are none and no error. It needs OpenZFS 2.3 for JSON output.
//
// The output is read without -L: as of OpenZFS 2.4, -L leaves the state out of
// devices that are disks. Paths are resolved here instead.
func (s *Storage) Pools(ctx context.Context) ([]Pool, error) {
	ctx, cancel := context.WithTimeout(ctx, poolTimeout)
	defer cancel()
	type result struct {
		out []byte
		err error
	}
	// Even a killed zpool can be stuck in the kernel on a suspended pool.
	done := make(chan result, 1)
	go func() {
		out, err := s.Run.Output(ctx, nil, "zpool", "status", "-j", "--json-int")
		done <- result{out, err}
	}()
	var r result
	select {
	case r = <-done:
	case <-ctx.Done():
		return nil, fmt.Errorf("zpool status did not answer in %s", poolTimeout)
	}
	if errors.Is(r.err, exec.ErrNotFound) {
		return nil, nil
	}
	if r.err != nil {
		var exit *ExitError
		if errors.As(r.err, &exit) && strings.Contains(exit.Output, "invalid option") {
			return nil, errors.New("reading pool health needs OpenZFS 2.3 or later")
		}
		return nil, r.err
	}
	if bytes.Contains(r.out, []byte("no pools available")) {
		return nil, nil
	}
	return parsePools(r.out)
}

// jsonVdevs keeps the vdevs in the order zpool lists them, which a map would
// lose.
type jsonVdevs []jsonVdev

func (v *jsonVdevs) UnmarshalJSON(data []byte) error {
	dec := json.NewDecoder(bytes.NewReader(data))
	if _, err := dec.Token(); err != nil { // {
		return err
	}
	for dec.More() {
		if _, err := dec.Token(); err != nil { // the key, repeated as its name
			return err
		}
		var one jsonVdev
		if err := dec.Decode(&one); err != nil {
			return err
		}
		*v = append(*v, one)
	}
	return nil
}

type jsonVdev struct {
	Name     string    `json:"name"`
	Type     string    `json:"vdev_type"`
	Class    string    `json:"class"`
	State    string    `json:"state"`
	Path     string    `json:"path"`
	Alloc    int64     `json:"alloc_space"`
	Total    int64     `json:"total_space"`
	Read     int64     `json:"read_errors"`
	Write    int64     `json:"write_errors"`
	Checksum int64     `json:"checksum_errors"`
	Vdevs    jsonVdevs `json:"vdevs"`
}

func (v jsonVdev) vdev() Vdev {
	out := Vdev{
		Name: v.Name, Type: v.Type, Class: v.Class, State: v.State, path: v.Path,
		Read: v.Read, Write: v.Write, Checksum: v.Checksum,
	}
	for _, child := range v.Vdevs {
		out.Children = append(out.Children, child.vdev())
	}
	return out
}

type jsonPool struct {
	Name   string    `json:"name"`
	State  string    `json:"state"`
	Status string    `json:"status"`
	Action string    `json:"action"`
	Errors int64     `json:"error_count"`
	Vdevs  jsonVdevs `json:"vdevs"` // one, the root
	Logs   jsonVdevs `json:"logs"`
	Cache  jsonVdevs `json:"l2cache"`
	Spares jsonVdevs `json:"spares"`
	Scan   *struct {
		Function  string `json:"function"`
		State     string `json:"state"`
		Start     int64  `json:"start_time"`
		End       int64  `json:"end_time"`
		ToExamine int64  `json:"to_examine"`
		Issued    int64  `json:"issued"`
		Processed int64  `json:"processed"`
		Errors    int64  `json:"errors"`
		Pause     int64  `json:"scrub_pause"`
	} `json:"scan_stats"`
}

func parsePools(data []byte) ([]Pool, error) {
	var doc struct {
		Pools map[string]jsonPool `json:"pools"`
	}
	if err := json.Unmarshal(data, &doc); err != nil {
		return nil, fmt.Errorf("reading zpool status: %w", err)
	}
	pools := make([]Pool, 0, len(doc.Pools))
	for _, p := range doc.Pools {
		pool := Pool{
			Name: p.Name, State: p.State, Errors: p.Errors,
			Status: strings.Join(strings.Fields(p.Status), " "),
			Action: strings.Join(strings.Fields(p.Action), " "),
			Vdevs:  []Vdev{},
		}
		for _, root := range p.Vdevs {
			pool.Size, pool.Alloc = root.Total, root.Alloc
			for _, v := range root.Vdevs {
				pool.Vdevs = append(pool.Vdevs, v.vdev())
			}
		}
		for _, group := range []jsonVdevs{p.Logs, p.Cache, p.Spares} {
			for _, v := range group {
				pool.Vdevs = append(pool.Vdevs, v.vdev())
			}
		}
		if scan := p.Scan; scan != nil {
			pool.Scan = &Scan{
				Function: scan.Function, State: scan.State, Start: scan.Start, End: scan.End,
				ToExamine: scan.ToExamine, Issued: scan.Issued, Repaired: scan.Processed,
				Errors: scan.Errors, Paused: scan.Pause > 0,
			}
		}
		pools = append(pools, pool)
	}
	slices.SortFunc(pools, func(a, b Pool) int { return strings.Compare(a.Name, b.Name) })
	return pools, nil
}

// LinkDisks fills in which of disks each device of pools is on, by following
// its path (/dev/disk/by-id/…-part1) to a disk or a partition of one. A device
// that is gone stays unlinked.
func (s *Storage) LinkDisks(pools []Pool, disks []Block) {
	owner := map[string]string{} // block name → disk name
	var walk func(disk string, blocks []Block)
	walk = func(disk string, blocks []Block) {
		for _, b := range blocks {
			owner[b.Name] = disk
			walk(disk, b.Children)
		}
	}
	for _, d := range disks {
		owner[d.Name] = d.Name
		walk(d.Name, d.Children)
	}
	var link func(vdevs []Vdev)
	link = func(vdevs []Vdev) {
		for i := range vdevs {
			v := &vdevs[i]
			link(v.Children)
			if v.path == "" {
				continue
			}
			if real, err := filepath.EvalSymlinks(s.Root + v.path); err == nil {
				v.Disk = owner[filepath.Base(real)]
			}
		}
	}
	for i := range pools {
		link(pools[i].Vdevs)
	}
}

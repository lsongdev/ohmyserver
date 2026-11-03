package system

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

// poolTimeout bounds asking zpool: on a suspended pool it can hang, and that
// must not hang the page or the request with it.
const poolTimeout = 5 * time.Second

// Pool is a storage pool: capacity from zpool list, and from zpool status what
// AddHealth adds.
type Pool struct {
	Name     string `json:"name"`
	Size     int64  `json:"size"`
	Alloc    int64  `json:"alloc"`
	Free     int64  `json:"free"`
	Capacity int    `json:"capacity"` // percent allocated
	Health   string `json:"health"`   // ONLINE, DEGRADED, FAULTED, …

	Status string `json:"status,omitempty"` // what is wrong, in zpool's words
	Action string `json:"action,omitempty"` // what to do about it
	Errors int64  `json:"errors"`           // files with known data errors
	Scan   *Scan  `json:"scan,omitempty"`   // the latest scrub or resilver
	Vdevs  []Vdev `json:"vdevs,omitempty"`  // in zpool's order: data, then log, cache, spare
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
	Size     int64  `json:"size,omitempty"` // of a device, not a group
	// Disk names the disk it is on (sdb), when it can be found now.
	Disk     string `json:"disk,omitempty"`
	path     string
	Children []Vdev `json:"children,omitempty"`
}

// bounded runs zpool for at most poolTimeout. Even a killed zpool can be stuck
// in the kernel on a suspended pool, so it does not wait for that either.
func (z *ZFS) bounded(ctx context.Context, args ...string) ([]byte, error) {
	ctx, cancel := context.WithTimeout(ctx, poolTimeout)
	type result struct {
		out []byte
		err error
	}
	done := make(chan result, 1)
	go func() {
		defer cancel()
		out, err := z.Run.Run(ctx, "zpool", args...)
		done <- result{out, err}
	}()
	select {
	case r := <-done:
		return r.out, r.err
	case <-ctx.Done():
		select {
		case r := <-done: // finished just then
			return r.out, r.err
		default:
		}
		return nil, fmt.Errorf("zpool did not answer in %s", poolTimeout)
	}
}

// Pools lists the imported pools with their capacity.
func (z *ZFS) Pools(ctx context.Context) ([]Pool, error) {
	out, err := z.bounded(ctx, "list", "-Hp", "-o", "name,size,alloc,free,cap,health")
	if err != nil {
		return nil, err
	}
	pools := []Pool{}
	for _, f := range rows(out, 6) {
		pools = append(pools, Pool{
			Name: f[0], Size: num(f[1]), Alloc: num(f[2]), Free: num(f[3]),
			Capacity: int(num(strings.TrimSuffix(f[4], "%"))), Health: f[5],
		})
	}
	return pools, nil
}

// AddHealth fills in the pools' devices and scrubs from zpool status, which
// needs OpenZFS 2.3 for JSON output. It is read without -L: as of OpenZFS 2.4,
// -L leaves the state out of devices that are disks. Paths are resolved here
// instead.
func (z *ZFS) AddHealth(ctx context.Context, pools []Pool) error {
	out, err := z.bounded(ctx, "status", "-j", "--json-int")
	if err != nil {
		var exit *ExitError
		if errors.As(err, &exit) && strings.Contains(exit.Output, "invalid option") {
			return errors.New("reading devices and scrubs needs OpenZFS 2.3 or later")
		}
		if errors.Is(err, exec.ErrNotFound) {
			return nil
		}
		return err
	}
	if bytes.Contains(out, []byte("no pools available")) {
		return nil
	}
	health, err := parseStatus(out)
	if err != nil {
		return err
	}
	for i := range pools {
		p := &pools[i]
		h, ok := health[p.Name]
		if !ok {
			continue
		}
		p.Health, p.Status, p.Action, p.Errors, p.Scan, p.Vdevs = h.Health, h.Status, h.Action, h.Errors, h.Scan, h.Vdevs
		z.linkDisks(p.Vdevs)
	}
	return nil
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
	Size     int64     `json:"phys_space"`
	Read     int64     `json:"read_errors"`
	Write    int64     `json:"write_errors"`
	Checksum int64     `json:"checksum_errors"`
	Vdevs    jsonVdevs `json:"vdevs"`
}

func (v jsonVdev) vdev() Vdev {
	out := Vdev{
		Name: v.Name, Type: v.Type, Class: v.Class, State: v.State, path: v.Path, Size: v.Size,
		Read: v.Read, Write: v.Write, Checksum: v.Checksum,
	}
	for _, child := range v.Vdevs {
		out.Children = append(out.Children, child.vdev())
	}
	return out
}

type jsonPool struct {
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

// parseStatus reads zpool status -j --json-int into pools by name.
func parseStatus(data []byte) (map[string]Pool, error) {
	var doc struct {
		Pools map[string]jsonPool `json:"pools"`
	}
	if err := json.Unmarshal(data, &doc); err != nil {
		return nil, fmt.Errorf("reading zpool status: %w", err)
	}
	pools := map[string]Pool{}
	for name, p := range doc.Pools {
		pool := Pool{
			Name: name, Health: p.State, Errors: p.Errors,
			Status: strings.Join(strings.Fields(p.Status), " "),
			Action: strings.Join(strings.Fields(p.Action), " "),
		}
		for _, root := range p.Vdevs {
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
		pools[name] = pool
	}
	return pools, nil
}

// linkDisks finds the disks of the devices that are disks: their path leads
// to a block device (/dev/disk/by-id/…-part1 → sdb1), and sysfs keeps a
// partition under its disk (…/block/sdb/sdb1). A device that is gone stays
// unlinked.
func (z *ZFS) linkDisks(vdevs []Vdev) {
	for i := range vdevs {
		v := &vdevs[i]
		z.linkDisks(v.Children)
		if v.Type != "disk" || v.path == "" {
			continue
		}
		device, err := filepath.EvalSymlinks(z.Root + v.path)
		if err != nil {
			continue
		}
		name := filepath.Base(device)
		sys, err := filepath.EvalSymlinks(filepath.Join(z.Root, "/sys/class/block", name))
		if err != nil {
			continue
		}
		if parent := filepath.Base(filepath.Dir(sys)); parent != "block" {
			name = parent
		}
		v.Disk = name
	}
}

// PoolCreated says when a pool was created.
func (z *ZFS) PoolCreated(ctx context.Context, pool string) (time.Time, error) {
	if !validPool(pool) {
		return time.Time{}, fmt.Errorf("%w pool name %q", ErrInvalid, pool)
	}
	out, err := z.bounded(ctx, "get", "-Hp", "-o", "value", "creation", pool)
	if err != nil {
		return time.Time{}, err
	}
	seconds := num(strings.TrimSpace(string(out)))
	if seconds == 0 {
		return time.Time{}, fmt.Errorf("reading when %s was created", pool)
	}
	return time.Unix(seconds, 0), nil
}

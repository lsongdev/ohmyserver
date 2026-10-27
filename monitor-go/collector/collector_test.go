package collector

import (
	"context"
	"testing"
	"time"

	"github.com/lsongdev/monitor-go/metrics"
)

// The local collector reads this machine; two readings give a sample.
func TestLocal(t *testing.T) {
	l := &Local{}
	ctx := context.Background()
	hosts, _ := l.Hosts(ctx)
	if len(hosts) != 1 || hosts[0].ID != "local" {
		t.Fatalf("hosts %+v", hosts)
	}
	a, info, err := l.Read(ctx, "local")
	if err != nil {
		t.Fatal(err)
	}
	time.Sleep(200 * time.Millisecond)
	b, _, err := l.Read(ctx, "local")
	if err != nil {
		t.Fatal(err)
	}
	s := metrics.Between(a, b)
	if b.CPUTotal <= 0 || b.MemTotal == 0 || b.MemUsed == 0 || len(b.Disks) == 0 || s.CPU < 0 || s.CPU > 100 || s.DiskTotal == 0 {
		t.Fatalf("reading %+v sample %+v", b, s)
	}
	if info.Hostname == "" || info.OS == "" || info.CPUCores == 0 {
		t.Fatalf("info %+v", info)
	}
	t.Logf("info %+v", info)
	t.Logf("disks %+v", b.Disks)
	t.Logf("sample %+v", s)
	if Kind("ssh:3") != "ssh" {
		t.Fatal("Kind")
	}
}

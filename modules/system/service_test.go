package system

import (
	"context"
	"errors"
	"runtime"
	"testing"
	"time"
)

func TestLocalServiceCombinesHostMetadataWithRuntimeInfo(t *testing.T) {
	started := time.Now().Add(-5 * time.Second)
	service := NewService(ServiceOptions{Info: Info{ProgramName: "test", Listen: ":8080"}, StartedAt: started})
	info, err := service.SystemInfo(context.Background(), false)
	if err != nil {
		t.Fatal(err)
	}
	if info.ProgramName != "test" || info.Listen != ":8080" || info.GoVersion != runtime.Version() || info.OS != runtime.GOOS || info.Arch != runtime.GOARCH || info.NumCPU != runtime.NumCPU() {
		t.Fatalf("unexpected system info: %+v", info)
	}
	if info.Uptime == "" {
		t.Fatal("system uptime was not computed")
	}
}

func TestLocalServicePassesContextToUpdateCapabilities(t *testing.T) {
	checkCalled, queueCalled := false, false
	service := NewService(ServiceOptions{
		Check: func(ctx context.Context) (UpdateInfo, error) {
			checkCalled = true
			return UpdateInfo{Checked: true, LatestVersion: "v2"}, ctx.Err()
		},
		Queue: func(ctx context.Context) error { queueCalled = true; return ctx.Err() },
	})
	info, err := service.SystemInfo(context.Background(), true)
	if err != nil || !checkCalled || !info.Update.Checked || info.Update.LatestVersion != "v2" {
		t.Fatalf("info=%+v err=%v checkCalled=%v", info, err, checkCalled)
	}
	if err := service.QueueSystemUpdate(context.Background()); err != nil || !queueCalled {
		t.Fatalf("queue err=%v queueCalled=%v", err, queueCalled)
	}
	canceled, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := service.SystemInfo(canceled, false); !errors.Is(err, context.Canceled) {
		t.Fatalf("canceled SystemInfo error=%v", err)
	}
}

func TestLocalServiceReportsUnavailableUpdateScheduler(t *testing.T) {
	service := NewService(ServiceOptions{})
	if err := service.QueueSystemUpdate(context.Background()); !errors.Is(err, ErrUpdateUnavailable) {
		t.Fatalf("QueueSystemUpdate error=%v", err)
	}
}

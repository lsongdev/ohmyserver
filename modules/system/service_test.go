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
	info, err := service.SystemInfo(context.Background())
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

func TestLocalServiceRespectsCanceledContext(t *testing.T) {
	canceled, cancel := context.WithCancel(context.Background())
	cancel()
	service := NewService(ServiceOptions{})
	if _, err := service.SystemInfo(canceled); !errors.Is(err, context.Canceled) {
		t.Fatalf("canceled SystemInfo error=%v", err)
	}
}

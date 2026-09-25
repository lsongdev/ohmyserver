package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	dashboard "github.com/lsongdev/ohmyserver/modules/dashboard"
	dashboardweb "github.com/lsongdev/ohmyserver/modules/dashboard/web"
	"github.com/shirou/gopsutil/v3/cpu"
	"github.com/shirou/gopsutil/v3/disk"
	"github.com/shirou/gopsutil/v3/host"
	"github.com/shirou/gopsutil/v3/load"
	"github.com/shirou/gopsutil/v3/mem"
)

type systemSource struct{}

func (systemSource) SystemSummary(ctx context.Context) (dashboard.SystemSummary, error) {
	if err := ctx.Err(); err != nil {
		return dashboard.SystemSummary{}, err
	}
	usage, err := cpu.PercentWithContext(ctx, time.Second, false)
	if err != nil {
		return dashboard.SystemSummary{}, err
	}
	cores, err := cpu.CountsWithContext(ctx, false)
	if err != nil {
		return dashboard.SystemSummary{}, err
	}
	memory, err := mem.VirtualMemoryWithContext(ctx)
	if err != nil {
		return dashboard.SystemSummary{}, err
	}
	filesystem, err := disk.UsageWithContext(ctx, "/")
	if err != nil {
		return dashboard.SystemSummary{}, err
	}
	info, err := host.InfoWithContext(ctx)
	if err != nil {
		return dashboard.SystemSummary{}, err
	}
	loadAverage, _ := load.AvgWithContext(ctx)
	result := dashboard.SystemSummary{
		Hostname: info.Hostname, Timestamp: time.Now().UTC().Format(time.RFC3339),
		CPUCores: cores, MemoryTotal: memory.Total, MemoryUsed: memory.Used, MemoryUsage: memory.UsedPercent,
		DiskTotal: filesystem.Total, DiskUsed: filesystem.Used, DiskUsage: filesystem.UsedPercent,
		Uptime: info.Uptime,
	}
	if len(usage) > 0 {
		result.CPUUsage = usage[0]
	}
	if loadAverage != nil {
		result.Load1, result.Load5, result.Load15 = loadAverage.Load1, loadAverage.Load5, loadAverage.Load15
	}
	if err := ctx.Err(); err != nil {
		return dashboard.SystemSummary{}, err
	}
	return result, nil
}

func main() {
	if err := run(os.Args[1:]); err != nil {
		log.Fatal(err)
	}
}

func run(args []string) error {
	if len(args) < 2 || args[0] != "web" || args[1] != "serve" {
		return fmt.Errorf("usage: dashboard web serve [--listen address]")
	}
	flags := flag.NewFlagSet("dashboard web serve", flag.ContinueOnError)
	listen := flags.String("listen", "127.0.0.1:8080", "HTTP listen address")
	if err := flags.Parse(args[2:]); err != nil {
		return err
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	service := dashboardweb.NewLocalService(nil, systemSource{})
	server := &http.Server{Addr: *listen, Handler: dashboardweb.New(service).StandaloneHandler(), ReadHeaderTimeout: 5 * time.Second}
	serveErr := make(chan error, 1)
	go func() { serveErr <- server.ListenAndServe() }()
	log.Printf("Dashboard module listening on http://%s", *listen)
	select {
	case err := <-serveErr:
		if errors.Is(err, http.ErrServerClosed) {
			return nil
		}
		return err
	case <-ctx.Done():
		shutdownCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 10*time.Second)
		defer cancel()
		return server.Shutdown(shutdownCtx)
	}
}

package main

import (
	"bytes"
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/lsongdev/ohmyserver/sysinfo"
)

func sendMetrics(serverURL, token string, payload *sysinfo.Metrics) error {
	body, err := json.Marshal(payload)
	if err != nil {
		return err
	}

	req, err := http.NewRequest(http.MethodPost, serverURL, bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")

	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusAccepted {
		return fmt.Errorf("unexpected status: %d", resp.StatusCode)
	}
	return nil
}

func main() {
	serverURL := flag.String("server", "http://localhost:9000/monitor/telegraf", "Server URL for metrics")
	interval := flag.Int("interval", 10, "Collection interval in seconds")
	token := flag.String("token", "", "Authentication token")
	flag.Parse()

	log.Printf("OhMyServer Agent starting")
	log.Printf("Server: %s", *serverURL)
	log.Printf("Interval: %ds", *interval)

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
	ticker := time.NewTicker(time.Duration(*interval) * time.Second)
	defer ticker.Stop()

	_ = token // reserved for future auth

	send := func() {
		stats, err := sysinfo.Collect()
		if err != nil {
			log.Printf("collect error: %v", err)
			return
		}
		if err := sendMetrics(*serverURL, *token, stats); err != nil {
			log.Printf("send error: %v", err)
			return
		}
		log.Printf("metrics sent: cpu=%.1f%% mem=%.1f%% disk=%.1f%%",
			stats.CPUUsage, stats.MemoryUsage, stats.DiskUsage)
	}

	send()

	for {
		select {
		case <-ticker.C:
			send()
		case sig := <-sigCh:
			log.Printf("received signal %v, exiting", sig)
			return
		}
	}
}

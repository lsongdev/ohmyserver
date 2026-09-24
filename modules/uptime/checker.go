package uptime

import (
	"context"
	"crypto/tls"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"os/exec"
	"regexp"
	"runtime"
	"strconv"
	"strings"
	"time"
)

type CheckRepository interface {
	Monitor(context.Context, int64) (*Monitor, error)
	InsertCheck(context.Context, *Check) error
}

type CheckEngine struct{ repository CheckRepository }

func NewChecker(repository CheckRepository) *CheckEngine { return &CheckEngine{repository: repository} }

// runPingCheck executes a ping check using system ping command
func runPingCheck(ctx context.Context, target string, timeoutSec int) (*Check, error) {
	var cmd *exec.Cmd
	if runtime.GOOS == "windows" {
		cmd = exec.CommandContext(ctx, "ping", "-n", "1", "-w", strconv.Itoa(timeoutSec*1000), target)
	} else if runtime.GOOS == "darwin" {
		cmd = exec.CommandContext(ctx, "ping", "-c", "1", "-W", strconv.Itoa(timeoutSec*1000), target)
	} else {
		// Linux
		cmd = exec.CommandContext(ctx, "ping", "-c", "1", "-W", strconv.Itoa(timeoutSec), target)
	}

	start := time.Now()
	output, err := cmd.CombinedOutput()
	duration := time.Since(start)

	check := &Check{
		Status:       "down",
		ResponseTime: duration.Milliseconds(),
		CheckedAt:    time.Now(),
	}

	if err != nil {
		check.Message = fmt.Sprintf("ping failed: %v", err)
		return check, nil
	}

	outputStr := string(output)

	// Parse response time from ping output
	// macOS/Linux: time=12.3 ms
	// Windows: time=12ms or time=12.3ms
	re := regexp.MustCompile(`time[=<>]([0-9.]+)\s*ms`)
	matches := re.FindStringSubmatch(outputStr)
	if len(matches) > 1 {
		if rt, err := strconv.ParseFloat(matches[1], 64); err == nil {
			check.ResponseTime = int64(rt)
		}
	}

	// Check for packet loss indicators
	if strings.Contains(outputStr, "100.0% packet loss") ||
		strings.Contains(outputStr, "100% packet loss") ||
		strings.Contains(outputStr, "Request timed out") ||
		strings.Contains(outputStr, "0 packets received") {
		check.Message = "Host unreachable"
		return check, nil
	}

	// On macOS, if ping succeeds but host is unknown
	if strings.Contains(outputStr, "unknown host") || strings.Contains(outputStr, "Name or service not known") {
		check.Message = "Unknown host"
		return check, nil
	}

	check.Status = "up"
	check.Message = fmt.Sprintf("Ping %dms", check.ResponseTime)
	return check, nil
}

// runTCPCheck checks if a TCP port is open
func runTCPCheck(ctx context.Context, target string, port int, timeoutSec int) (*Check, error) {
	address := net.JoinHostPort(target, strconv.Itoa(port))
	if port == 0 {
		address = target
	}

	start := time.Now()
	conn, err := (&net.Dialer{Timeout: time.Duration(timeoutSec) * time.Second}).DialContext(ctx, "tcp", address)
	duration := time.Since(start)

	check := &Check{
		ResponseTime: duration.Milliseconds(),
		CheckedAt:    time.Now(),
	}

	if err != nil {
		check.Status = "down"
		check.Message = err.Error()
		return check, nil
	}
	defer conn.Close()

	check.Status = "up"
	check.Message = fmt.Sprintf("Connected in %dms", check.ResponseTime)
	return check, nil
}

// runHTTPCheck performs an HTTP/HTTPS request
func runHTTPCheck(ctx context.Context, target string, timeoutSec int, expectedStatus int) (*Check, error) {
	// Ensure target has scheme
	checkURL := target
	if !strings.HasPrefix(checkURL, "http://") && !strings.HasPrefix(checkURL, "https://") {
		checkURL = "http://" + checkURL
	}

	client := &http.Client{
		Timeout: time.Duration(timeoutSec) * time.Second,
		Transport: &http.Transport{
			TLSClientConfig: &tls.Config{InsecureSkipVerify: true},
		},
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			if len(via) >= 10 {
				return fmt.Errorf("too many redirects")
			}
			return nil
		},
	}

	start := time.Now()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, checkURL, nil)
	if err != nil {
		return nil, err
	}
	resp, err := client.Do(req)
	duration := time.Since(start)

	check := &Check{
		ResponseTime: duration.Milliseconds(),
		CheckedAt:    time.Now(),
	}

	if err != nil {
		check.Status = "down"
		if urlErr, ok := err.(*url.Error); ok && urlErr.Timeout() {
			check.Message = "Request timed out"
		} else {
			check.Message = err.Error()
		}
		return check, nil
	}
	defer resp.Body.Close()
	io.Copy(io.Discard, resp.Body)

	check.StatusCode = resp.StatusCode

	if expectedStatus > 0 && resp.StatusCode != expectedStatus {
		check.Status = "down"
		check.Message = fmt.Sprintf("Unexpected status: %d (expected %d)", resp.StatusCode, expectedStatus)
		return check, nil
	}

	if resp.StatusCode >= 200 && resp.StatusCode < 400 {
		check.Status = "up"
		check.Message = fmt.Sprintf("HTTP %d in %dms", resp.StatusCode, check.ResponseTime)
	} else {
		check.Status = "down"
		check.Message = fmt.Sprintf("HTTP %d", resp.StatusCode)
	}

	return check, nil
}

// RunCheck executes a check for a monitor
func (s *CheckEngine) RunCheck(ctx context.Context, monitor *Monitor) (*Check, error) {
	timeout := monitor.Timeout
	if timeout < 1 {
		timeout = 10
	}
	ctx, cancel := context.WithTimeout(ctx, time.Duration(timeout)*time.Second)
	defer cancel()
	var check *Check
	var err error

	switch monitor.Type {
	case Ping:
		check, err = runPingCheck(ctx, monitor.Target, timeout)
	case TCP:
		check, err = runTCPCheck(ctx, monitor.Target, monitor.Port, timeout)
	case HTTP:
		check, err = runHTTPCheck(ctx, monitor.Target, timeout, monitor.ExpectedStatus)
	default:
		return nil, fmt.Errorf("unknown monitor type: %s", monitor.Type)
	}

	if err != nil {
		return nil, err
	}

	check.MonitorID = monitor.ID
	return check, nil
}

func (s *CheckEngine) CheckNow(ctx context.Context, id int64) (*Check, error) {
	monitor, err := s.repository.Monitor(ctx, id)
	if err != nil {
		return nil, err
	}
	check, err := s.RunCheck(ctx, monitor)
	if err != nil {
		return nil, err
	}
	if err := s.repository.InsertCheck(ctx, check); err != nil {
		return nil, err
	}
	return check, nil
}

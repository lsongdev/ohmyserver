package api

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/netip"
	"os/exec"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/lsongdev/ohmyserver/modules/netmgr/system"
)

const toolTimeout = 90 * time.Second

var hostName = regexp.MustCompile(`^[A-Za-z0-9_]([A-Za-z0-9._-]{0,251}[A-Za-z0-9_.])?$`)

// validTarget accepts an IP address or host name, and so never anything a
// command could read as an option.
func validTarget(s string) bool {
	if _, err := netip.ParseAddr(s); err == nil {
		return true
	}
	return hostName.MatchString(s)
}

type toolRequest struct {
	Tool   string `json:"tool"` // ping, trace, lookup or port
	Target string `json:"target"`
	Port   int    `json:"port,omitempty"`   // for port
	Record string `json:"record,omitempty"` // for lookup: A, AAAA, MX, TXT, NS, CNAME or PTR
}

// flushWriter sends each write to the client at once, for output to appear
// as a command produces it.
type flushWriter struct {
	w http.ResponseWriter
	c *http.ResponseController
}

func (f flushWriter) Write(p []byte) (int, error) {
	n, err := f.w.Write(p)
	f.c.Flush() // not all writers can; the output then arrives at the end
	return n, err
}

// runTool streams a diagnostic's output as plain text. Closing the request
// stops it.
func (s *Service) runTool(w http.ResponseWriter, r *http.Request) {
	if !s.installed {
		writeError(w, http.StatusServiceUnavailable, "unsupported", "iproute2 is not installed on this machine", nil)
		return
	}
	var req toolRequest
	if err := decode(r, &req); err != nil {
		respond(w, nil, err)
		return
	}
	req.Target = strings.TrimSpace(req.Target)
	if !validTarget(req.Target) {
		writeError(w, http.StatusUnprocessableEntity, "invalid", "Enter a host name or IP address", map[string]string{"target": "Enter a host name or IP address"})
		return
	}
	var run func(context.Context, io.Writer) error
	switch req.Tool {
	case "ping":
		run = func(ctx context.Context, out io.Writer) error {
			return system.Stream(ctx, out, "ping", "-n", "-c", "4", "-W", "2", req.Target)
		}
	case "trace":
		run = func(ctx context.Context, out io.Writer) error {
			if _, err := exec.LookPath("traceroute"); err == nil {
				return system.Stream(ctx, out, "traceroute", "-n", "-w", "2", "-q", "1", "-m", "30", req.Target)
			}
			return system.Stream(ctx, out, "tracepath", "-n", req.Target)
		}
	case "lookup":
		run = func(ctx context.Context, out io.Writer) error { return lookup(ctx, out, req.Target, req.Record) }
	case "port":
		if req.Port < 1 || req.Port > 65535 {
			writeError(w, http.StatusUnprocessableEntity, "invalid", "Enter a port from 1 to 65535", map[string]string{"port": "Enter a port from 1 to 65535"})
			return
		}
		run = func(ctx context.Context, out io.Writer) error { return dialPort(ctx, out, req.Target, req.Port) }
	default:
		writeError(w, http.StatusBadRequest, "bad_request", "unknown tool", nil)
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), toolTimeout)
	defer cancel()
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.WriteHeader(http.StatusOK)
	out := flushWriter{w, http.NewResponseController(w)}
	if err := run(ctx, out); err != nil && r.Context().Err() == nil {
		var exit *exec.ExitError
		switch {
		case errors.Is(ctx.Err(), context.DeadlineExceeded):
			fmt.Fprintf(out, "\nStopped after %s.\n", toolTimeout)
		case errors.As(err, &exit):
			fmt.Fprintf(out, "\nFinished with exit status %d.\n", exit.ExitCode())
		default:
			fmt.Fprintf(out, "%v\n", err)
		}
	}
}

func dialPort(ctx context.Context, out io.Writer, host string, port int) error {
	address := net.JoinHostPort(host, strconv.Itoa(port))
	dialCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	start := time.Now()
	conn, err := new(net.Dialer).DialContext(dialCtx, "tcp", address)
	if err != nil {
		return fmt.Errorf("%s: cannot connect: %w", address, err)
	}
	defer conn.Close()
	_, err = fmt.Fprintf(out, "%s: open, connected to %s in %s\n", address, conn.RemoteAddr(), time.Since(start).Round(time.Millisecond))
	return err
}

func lookup(ctx context.Context, out io.Writer, name, record string) error {
	r := net.DefaultResolver
	print := func(values ...string) {
		for _, v := range values {
			fmt.Fprintln(out, v)
		}
	}
	switch record {
	case "", "A", "AAAA":
		network := map[string]string{"": "ip", "A": "ip4", "AAAA": "ip6"}[record]
		ips, err := r.LookupIP(ctx, network, name)
		if err != nil {
			return err
		}
		for _, ip := range ips {
			print(ip.String())
		}
	case "CNAME":
		cname, err := r.LookupCNAME(ctx, name)
		if err != nil {
			return err
		}
		print(cname)
	case "MX":
		mx, err := r.LookupMX(ctx, name)
		if err != nil {
			return err
		}
		for _, m := range mx {
			print(fmt.Sprintf("%d %s", m.Pref, m.Host))
		}
	case "NS":
		ns, err := r.LookupNS(ctx, name)
		if err != nil {
			return err
		}
		for _, n := range ns {
			print(n.Host)
		}
	case "TXT":
		txt, err := r.LookupTXT(ctx, name)
		if err != nil {
			return err
		}
		print(txt...)
	case "PTR":
		names, err := r.LookupAddr(ctx, name)
		if err != nil {
			return err
		}
		print(names...)
	default:
		return fmt.Errorf("unknown record type %s", record)
	}
	return nil
}

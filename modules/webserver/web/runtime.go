package web

import (
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	webservermodule "github.com/lsongdev/ohmyserver/modules/webserver"
	"github.com/lsongdev/ohmyserver/modules/webserver/web/repository"
	"github.com/lsongdev/yabber"
)

// Runtime owns the Web Server module's reverse-proxy lifecycle. The host
// supplies certificate material and ACME HTTP-01 tokens through narrow
// capabilities; all listener, host, route, and policy configuration comes
// from this module's repository.
type Runtime struct {
	ctx          context.Context
	repository   *repository.Repository
	certificates webservermodule.CertificateSource
	http01       webservermodule.HTTP01Store
	mu           sync.Mutex
	instance     *yabber.Server
}

func NewRuntime(ctx context.Context, repo *repository.Repository, certificates webservermodule.CertificateSource, http01 webservermodule.HTTP01Store) *Runtime {
	if ctx == nil {
		panic("webserver: runtime context is required")
	}
	return &Runtime{ctx: ctx, repository: repo, certificates: certificates, http01: http01}
}

func (r *Runtime) Apply(ctx context.Context) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	config, err := r.BuildConfig(ctx)
	if err != nil {
		return err
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if err := r.ctx.Err(); err != nil {
		return err
	}
	if r.instance == nil {
		options := []yabber.Option{yabber.WithCertificateLoader(certificateLoader{ctx: r.ctx, source: r.certificates})}
		if r.http01 != nil {
			options = append(options, yabber.WithHTTP01Store(r.http01))
		}
		instance, err := yabber.New(config, options...)
		if err != nil {
			return err
		}
		if err := instance.Start(); err != nil {
			return err
		}
		r.instance = instance
		return nil
	}
	if err := r.instance.Apply(config); err != nil {
		if !errors.Is(err, yabber.ErrRestartRequired) {
			return err
		}
		restartCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 10*time.Second)
		defer cancel()
		return r.instance.Restart(restartCtx, config)
	}
	return nil
}

func (r *Runtime) Close() error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.instance == nil {
		return nil
	}
	err := r.instance.Close()
	r.instance = nil
	return err
}

func (r *Runtime) MetricsHandler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		r.mu.Lock()
		instance := r.instance
		r.mu.Unlock()
		if instance == nil {
			http.Error(w, "web server is stopped", http.StatusServiceUnavailable)
			return
		}
		instance.MetricsHandler().ServeHTTP(w, req)
	})
}

func (r *Runtime) BuildConfig(ctx context.Context) (yabber.Config, error) {
	listeners, err := r.repository.ListListeners(ctx)
	if err != nil {
		return yabber.Config{}, err
	}
	hosts, err := r.repository.ListHosts(ctx)
	if err != nil {
		return yabber.Config{}, err
	}
	policies, err := r.repository.ListPolicies(ctx)
	if err != nil {
		return yabber.Config{}, err
	}
	config := yabber.Config{Version: yabber.ConfigVersion, Server: yabber.ServerConfig{
		ShutdownTimeout: 10 * time.Second, ReadTimeout: 30 * time.Second,
		ReadHeaderTimeout: 5 * time.Second, IdleTimeout: 60 * time.Second,
		MaxHeaderBytes: 1 << 20,
	}}
	for _, policy := range policies {
		item := yabber.Policy{Name: policy.Name}
		if policy.MaxBodyBytes > 0 || policy.RequestTimeoutSeconds > 0 {
			item.Request = &yabber.RequestPolicy{MaxBodyBytes: policy.MaxBodyBytes, Timeout: time.Duration(policy.RequestTimeoutSeconds) * time.Second}
		}
		if policy.RateRequestsPerSecond > 0 {
			item.RateLimit = &yabber.RateLimitPolicy{RequestsPerSecond: policy.RateRequestsPerSecond, Burst: policy.RateBurst, Key: policy.RateKey, MaxKeys: policy.RateMaxKeys, IdleTimeout: time.Duration(policy.RateIdleTimeoutSeconds) * time.Second}
		}
		if policy.ConcurrencyMax > 0 {
			item.Concurrency = &yabber.ConcurrencyPolicy{Max: policy.ConcurrencyMax}
		}
		config.Policies = append(config.Policies, item)
	}
	certificateNames := make(map[string]struct{})
	for _, listener := range listeners {
		enabled := listener.Enabled
		item := yabber.Listener{Name: listener.Name, Address: listener.Address, Protocol: yabber.Protocol(listener.Protocol), Enabled: &enabled, TrustedProxies: splitTokens(listener.TrustedProxies), Policies: listener.PolicyNames}
		if listener.Protocol == string(yabber.ProtocolHTTPS) {
			item.TLS = &yabber.TLSConfig{MinVersion: listener.MinTLSVersion, DefaultCertificate: certificateReference(listener.CertificateID)}
			if item.TLS.DefaultCertificate != "" && listener.Enabled {
				certificateNames[item.TLS.DefaultCertificate] = struct{}{}
			}
		}
		config.Listeners = append(config.Listeners, item)
	}
	for _, host := range hosts {
		enabled := host.Enabled
		item := yabber.VirtualHost{Name: host.Name, Hostnames: splitTokens(host.Hostnames), Listeners: host.ListenerNames, Enabled: &enabled, HTTP: &yabber.HostHTTP{RedirectToHTTPS: host.ForceHTTPS}, Policies: host.PolicyNames}
		if host.CertificateID != nil {
			name := certificateReference(host.CertificateID)
			item.TLS = &yabber.HostTLS{Certificate: name}
			if host.Enabled {
				certificateNames[name] = struct{}{}
			}
		}
		routes, err := r.repository.ListRoutesByHost(ctx, host.ID)
		if err != nil {
			return yabber.Config{}, err
		}
		for _, route := range routes {
			routeEnabled := route.Enabled
			out := yabber.Route{Name: route.Name, Enabled: &routeEnabled, Match: yabber.Match{Method: route.Method, Path: route.Path}, Policies: route.PolicyNames}
			switch route.HandlerType {
			case "reverse_proxy":
				out.Handle.ReverseProxy = &yabber.ReverseProxy{URL: route.UpstreamURL, StripPrefix: route.StripPrefix, RequestTimeout: time.Duration(route.RequestTimeoutSeconds) * time.Second, ResponseHeaderTimeout: time.Duration(route.ResponseHeaderTimeoutSeconds) * time.Second, MaxConnsPerHost: route.MaxConnsPerHost}
			case "file_server":
				out.Handle.FileServer = &yabber.FileServer{Root: route.Root, StripPrefix: route.StripPrefix}
			case "static_response":
				out.Handle.StaticResponse = &yabber.StaticResponse{Status: route.ResponseStatus, Body: route.ResponseBody}
			case "redirect":
				out.Handle.Redirect = &yabber.Redirect{Location: route.RedirectLocation, Status: route.RedirectStatus}
			case "worker":
				out.Handle.Worker = &yabber.WorkerHandler{Script: route.WorkerScript}
			default:
				return yabber.Config{}, fmt.Errorf("route %q has invalid handler type %q", route.Name, route.HandlerType)
			}
			item.Routes = append(item.Routes, out)
		}
		config.Hosts = append(config.Hosts, item)
	}
	if r.certificates != nil {
		summaries, err := r.certificates.ListSummaries(ctx)
		if err != nil {
			return yabber.Config{}, err
		}
		active := make(map[int64]bool, len(summaries))
		for _, summary := range summaries {
			active[summary.ID] = summary.Status == "active"
		}
		for name := range certificateNames {
			id, err := strconv.ParseInt(strings.TrimPrefix(name, "certificate:"), 10, 64)
			if err != nil || !active[id] {
				return yabber.Config{}, fmt.Errorf("certificate %q is not active", name)
			}
			config.Certificates = append(config.Certificates, yabber.Certificate{Name: name})
		}
	} else if len(certificateNames) != 0 {
		return yabber.Config{}, errors.New("certificate source is unavailable")
	}
	return config, nil
}

type certificateLoader struct {
	ctx    context.Context
	source webservermodule.CertificateSource
}

func (l certificateLoader) LoadCertificate(definition yabber.Certificate) (*tls.Certificate, error) {
	if l.source == nil {
		return nil, errors.New("certificate source is unavailable")
	}
	id, err := strconv.ParseInt(strings.TrimPrefix(definition.Name, "certificate:"), 10, 64)
	if err != nil {
		return nil, fmt.Errorf("invalid certificate reference %q", definition.Name)
	}
	material, err := l.source.LoadTLSMaterial(l.ctx, id)
	if err != nil {
		return nil, err
	}
	if material.Status != "active" || material.CertificatePEM == "" {
		return nil, fmt.Errorf("certificate %d is not active", id)
	}
	loaded, err := tls.X509KeyPair([]byte(material.CertificatePEM), []byte(material.PrivateKeyPEM))
	if err != nil {
		return nil, err
	}
	return &loaded, nil
}

func certificateReference(id *int64) string {
	if id == nil {
		return ""
	}
	return fmt.Sprintf("certificate:%d", *id)
}

func splitTokens(value string) []string {
	return strings.FieldsFunc(value, func(r rune) bool {
		return r == ',' || r == '\n' || r == '\r' || r == ' ' || r == '\t'
	})
}

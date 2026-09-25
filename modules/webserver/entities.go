package webserver

import (
	"context"
	"time"
)

type Listener struct {
	ID              int64    `json:"id"`
	Name            string   `json:"name"`
	Address         string   `json:"address"`
	Protocol        string   `json:"protocol"`
	Enabled         bool     `json:"enabled"`
	MinTLSVersion   string   `json:"min_tls_version"`
	CertificateID   *int64   `json:"certificate_id"`
	CertificateName string   `json:"certificate_name"`
	TrustedProxies  string   `json:"trusted_proxies"`
	PolicyIDs       []int64  `json:"policy_ids"`
	PolicyNames     []string `json:"policy_names"`
}
type Host struct {
	ID              int64     `json:"id"`
	Name            string    `json:"name"`
	Hostnames       string    `json:"hostnames"`
	Enabled         bool      `json:"enabled"`
	CertificateID   *int64    `json:"certificate_id"`
	CertificateName string    `json:"certificate_name"`
	ForceHTTPS      bool      `json:"force_https"`
	ListenerIDs     []int64   `json:"listener_ids"`
	ListenerNames   []string  `json:"listener_names"`
	PolicyNames     []string  `json:"policy_names"`
	PolicyIDs       []int64   `json:"policy_ids"`
	Routes          []Route   `json:"routes"`
	CreatedAt       time.Time `json:"created_at"`
	UpdatedAt       time.Time `json:"updated_at"`
}
type Route struct {
	ID                           int64     `json:"id"`
	Name                         string    `json:"name"`
	HostID                       int64     `json:"host_id"`
	HostName                     string    `json:"host_name"`
	Method                       string    `json:"method"`
	Path                         string    `json:"path"`
	HandlerType                  string    `json:"handler_type"`
	UpstreamURL                  string    `json:"upstream_url"`
	Enabled                      bool      `json:"enabled"`
	Root                         string    `json:"root"`
	StripPrefix                  string    `json:"strip_prefix"`
	ResponseStatus               int       `json:"response_status"`
	ResponseBody                 string    `json:"response_body"`
	RedirectLocation             string    `json:"redirect_location"`
	RedirectStatus               int       `json:"redirect_status"`
	WorkerScript                 string    `json:"worker_script"`
	RequestTimeoutSeconds        int       `json:"request_timeout_seconds"`
	ResponseHeaderTimeoutSeconds int       `json:"response_header_timeout_seconds"`
	MaxConnsPerHost              int       `json:"max_conns_per_host"`
	PolicyIDs                    []int64   `json:"policy_ids"`
	PolicyNames                  []string  `json:"policy_names"`
	CreatedAt                    time.Time `json:"created_at"`
	UpdatedAt                    time.Time `json:"updated_at"`
}
type Policy struct {
	ID                     int64   `json:"id"`
	Name                   string  `json:"name"`
	MaxBodyBytes           int64   `json:"max_body_bytes"`
	RequestTimeoutSeconds  int     `json:"request_timeout_seconds"`
	RateRequestsPerSecond  float64 `json:"rate_requests_per_second"`
	RateBurst              int     `json:"rate_burst"`
	RateKey                string  `json:"rate_key"`
	RateMaxKeys            int     `json:"rate_max_keys"`
	RateIdleTimeoutSeconds int     `json:"rate_idle_timeout_seconds"`
	ConcurrencyMax         int     `json:"concurrency_max"`
}
type Certificate struct {
	ID      int64  `json:"id"`
	Domains string `json:"domains"`
}

type CertificateSummary struct {
	ID      int64
	Domains string
	Status  string
}

type TLSMaterial struct {
	CertificatePEM string
	PrivateKeyPEM  string
	Status         string
}

// CertificateSource lets the host provide ACME-owned summaries and TLS
// material without giving this module access to ACME storage.
type CertificateSource interface {
	ListSummaries(context.Context) ([]CertificateSummary, error)
	LoadTLSMaterial(context.Context, int64) (TLSMaterial, error)
}

type HTTP01Store interface {
	GetHTTP01Token(host, token string) (string, bool)
}
type Service interface {
	WebListeners(context.Context) ([]Listener, error)
	WebHosts(context.Context) ([]Host, error)
	WebPolicies(context.Context) ([]Policy, error)
	CreateWebListenerAPI(context.Context, Listener) (Listener, error)
	UpdateWebListenerAPI(context.Context, int64, Listener) error
	DeleteWebListenerAPI(context.Context, int64) error
	WebCertificates(context.Context) ([]Certificate, error)
	CreateWebHostAPI(context.Context, Host) (Host, error)
	UpdateWebHostAPI(context.Context, int64, Host) error
	DeleteWebHostAPI(context.Context, int64) error
	CreateWebRouteAPI(context.Context, int64, Route) (Route, error)
	UpdateWebRouteAPI(context.Context, int64, Route) error
	DeleteWebRouteAPI(context.Context, int64) error
	CreateWebPolicyAPI(context.Context, Policy) (Policy, error)
	UpdateWebPolicyAPI(context.Context, int64, Policy) error
	DeleteWebPolicyAPI(context.Context, int64) error
}

// Package dns defines the public data contracts for the DNS module.
package dns

import "time"

type Provider struct {
	ID        int64     `json:"id"`
	Name      string    `json:"name"`
	Type      string    `json:"type"`
	Config    string    `json:"-"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}
type ProviderSummary struct {
	ID        int64     `json:"id"`
	Name      string    `json:"name"`
	Type      string    `json:"type"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}
type Settings struct {
	Enabled      bool      `json:"enabled"`
	UDPAddr      string    `json:"udp_addr"`
	TCPAddr      string    `json:"tcp_addr"`
	DoTAddr      string    `json:"dot_addr"`
	DoHAddr      string    `json:"doh_addr"`
	CertFile     string    `json:"cert_file"`
	KeyFile      string    `json:"key_file"`
	CacheEnabled bool      `json:"cache_enabled"`
	CacheMinTTL  int       `json:"cache_min_ttl"`
	CacheMaxTTL  int       `json:"cache_max_ttl"`
	CacheEntries int       `json:"cache_entries"`
	Upstreams    string    `json:"upstreams"`
	UpdatedAt    time.Time `json:"updated_at"`
}
type Zone struct {
	ID          int64     `json:"id"`
	Name        string    `json:"name"`
	Description string    `json:"description"`
	Enabled     bool      `json:"enabled"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
}
type LocalRecord struct {
	ID        int64     `json:"id"`
	ZoneID    int64     `json:"zone_id"`
	Name      string    `json:"name"`
	Type      string    `json:"type"`
	TTL       int       `json:"ttl"`
	Value     string    `json:"value"`
	Priority  int       `json:"priority"`
	Weight    int       `json:"weight"`
	Port      int       `json:"port"`
	Enabled   bool      `json:"enabled"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}
type Subscription struct {
	ID           int64      `json:"id"`
	Name         string     `json:"name"`
	Kind         string     `json:"kind"`
	URL          string     `json:"url"`
	Enabled      bool       `json:"enabled"`
	RefreshHours int        `json:"refresh_hours"`
	LocalPath    string     `json:"local_path"`
	LastUpdated  *time.Time `json:"last_updated"`
	LastError    string     `json:"last_error"`
	CreatedAt    time.Time  `json:"created_at"`
	UpdatedAt    time.Time  `json:"updated_at"`
}
type Rule struct {
	ID        int64     `json:"id"`
	Rule      string    `json:"rule"`
	Enabled   bool      `json:"enabled"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}
type ProviderZone struct {
	ID        string `json:"id"`
	Name      string `json:"name"`
	CreatedAt string `json:"created_at"`
}
type ProviderRecord struct {
	ID        string `json:"id"`
	Type      string `json:"type"`
	Name      string `json:"name"`
	Value     string `json:"value"`
	TTL       int    `json:"ttl"`
	Comment   string `json:"comment"`
	UpdatedAt string `json:"updated_at"`
}

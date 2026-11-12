// Package provider describes DNS hosting services (Cloudflare, …) through
// one interface: the zones an account holds and their records. Each
// service lives in a subpackage and registers itself as a Kind.
package provider

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strings"
	"sync"
)

// Zone is a domain hosted by the service.
type Zone struct {
	ID          string   `json:"id"`
	Name        string   `json:"name"` // example.com, without the final dot
	Status      string   `json:"status,omitempty"`
	NameServers []string `json:"name_servers,omitempty"`
	MinTTL      int      `json:"min_ttl,omitempty"` // account/zone-specific TTL floor, when known
}

// Record is a DNS record in a zone. Name is the full name
// (www.example.com).
type Record struct {
	ID       string `json:"id,omitempty"`
	Type     string `json:"type"` // A, AAAA, CNAME, TXT, MX, …
	Name     string `json:"name"`
	Content  string `json:"content"`
	TTL      int    `json:"ttl"`                // seconds; 1 means automatic where supported
	Priority *int   `json:"priority,omitempty"` // MX, SRV
	// Proxied routes traffic through the service's proxy (Cloudflare's
	// orange cloud); nil where the service has no such thing.
	Proxied *bool  `json:"proxied,omitempty"`
	Comment string `json:"comment,omitempty"`
}

// Filter selects records; empty fields match everything.
type Filter struct {
	Type string
	Name string // full name, exact
}

// Provider is an account at a DNS hosting service.
type Provider interface {
	Zones(ctx context.Context) ([]Zone, error)
	Records(ctx context.Context, zoneID string, f Filter) ([]Record, error)
	CreateRecord(ctx context.Context, zoneID string, r Record) (Record, error)
	UpdateRecord(ctx context.Context, zoneID string, r Record) (Record, error)
	DeleteRecord(ctx context.Context, zoneID, recordID string) error
}

// Field is a credential a kind needs, for the account form.
type Field struct {
	Name     string `json:"name"`
	Label    string `json:"label"`
	Secret   bool   `json:"secret"`
	Hint     string `json:"hint,omitempty"`
	Optional bool   `json:"optional,omitempty"`
}

// Kind is a DNS hosting service.
type Kind struct {
	ID     string  `json:"id"`
	Title  string  `json:"title"`
	Fields []Field `json:"fields"`
	// Help says where to get the credentials.
	Help string `json:"help,omitempty"`
	// Features the service's records have.
	Proxy       bool     `json:"proxy"`                  // records can be proxied
	Comments    bool     `json:"comments"`               // record comments can be edited
	MinTTL      int      `json:"min_ttl"`                // zero means automatic TTL is supported
	RecordTypes []string `json:"record_types,omitempty"` // empty uses the module's types
	// New opens an account from its credentials.
	New func(credentials map[string]string) (Provider, error) `json:"-"`
}

var (
	mu    sync.RWMutex
	kinds = map[string]Kind{}
)

// Register adds a kind; subpackages call it from init.
func Register(k Kind) {
	mu.Lock()
	defer mu.Unlock()
	kinds[k.ID] = k
}

// Kinds lists the registered kinds by title.
func Kinds() []Kind {
	mu.RLock()
	defer mu.RUnlock()
	list := make([]Kind, 0, len(kinds))
	for _, k := range kinds {
		list = append(list, k)
	}
	sort.Slice(list, func(i, j int) bool { return list[i].Title < list[j].Title })
	return list
}

// ErrInvalid wraps rejected input.
var ErrInvalid = errors.New("invalid")

// Open returns the account of kind with credentials, checking required
// fields. The kind validates alternative credential combinations.
func Open(kind string, credentials map[string]string) (Provider, error) {
	mu.RLock()
	k, ok := kinds[kind]
	mu.RUnlock()
	if !ok {
		return nil, fmt.Errorf("%w: unknown provider %q", ErrInvalid, kind)
	}
	for _, f := range k.Fields {
		if !f.Optional && strings.TrimSpace(credentials[f.Name]) == "" {
			return nil, fmt.Errorf("%w: %s is required", ErrInvalid, f.Label)
		}
	}
	return k.New(credentials)
}

// Lookup returns a registered kind.
func Lookup(kind string) (Kind, bool) {
	mu.RLock()
	defer mu.RUnlock()
	k, ok := kinds[kind]
	return k, ok
}

// Fqdn normalizes a name: lower case, no final dot.
func Fqdn(name string) string {
	return strings.ToLower(strings.TrimSuffix(strings.TrimSpace(name), "."))
}

// InZone reports whether name is zone or below it.
func InZone(name, zone string) bool {
	name, zone = Fqdn(name), Fqdn(zone)
	return name == zone || strings.HasSuffix(name, "."+zone)
}

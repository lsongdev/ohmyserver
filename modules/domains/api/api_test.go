package api

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"sync"
	"testing"

	"github.com/lsongdev/ohmyserver/modules/domains/provider"
	"github.com/lsongdev/ohmyserver/modules/domains/store"
)

// memory is a provider holding zones in memory; the token "good" works.
type memory struct {
	mu      sync.Mutex
	zones   []provider.Zone
	records map[string][]provider.Record // by zone ID
	seq     int
}

var shared = &memory{
	zones:   []provider.Zone{{ID: "z1", Name: "example.com"}, {ID: "z2", Name: "dev.example.com"}},
	records: map[string][]provider.Record{},
}

func init() {
	provider.Register(provider.Kind{ID: "memory", Title: "Memory", Fields: []provider.Field{{Name: "token", Label: "Token", Secret: true}},
		New: func(c map[string]string) (provider.Provider, error) {
			if c["token"] != "good" {
				return badToken{}, nil
			}
			return shared, nil
		}})
}

type badToken struct{ provider.Provider }

func (badToken) Zones(context.Context) ([]provider.Zone, error) {
	return nil, errors.New("invalid token")
}

func (m *memory) Zones(context.Context) ([]provider.Zone, error) { return m.zones, nil }
func (m *memory) Records(_ context.Context, zone string, f provider.Filter) ([]provider.Record, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	var out []provider.Record
	for _, r := range m.records[zone] {
		if (f.Type == "" || f.Type == r.Type) && (f.Name == "" || f.Name == r.Name) {
			out = append(out, r)
		}
	}
	return out, nil
}
func (m *memory) CreateRecord(_ context.Context, zone string, r provider.Record) (provider.Record, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.seq++
	r.ID = fmt.Sprint(m.seq)
	m.records[zone] = append(m.records[zone], r)
	return r, nil
}
func (m *memory) UpdateRecord(_ context.Context, zone string, r provider.Record) (provider.Record, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	for i, old := range m.records[zone] {
		if old.ID == r.ID {
			m.records[zone][i] = r
		}
	}
	return r, nil
}
func (m *memory) DeleteRecord(_ context.Context, zone, id string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	list := m.records[zone][:0]
	for _, r := range m.records[zone] {
		if r.ID != id {
			list = append(list, r)
		}
	}
	m.records[zone] = list
	return nil
}

func newService(t *testing.T) *Service {
	t.Helper()
	st, err := store.Open(context.Background(), filepath.Join(t.TempDir(), "domains.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { st.Close() })
	return NewService(st, nil)
}

func TestAccountsAndRecords(t *testing.T) {
	ctx := context.Background()
	s := newService(t)
	var pe *ProviderError
	if _, err := s.CreateAccount(ctx, AccountInput{Kind: "memory", Credentials: map[string]string{"token": "bad"}}); !errors.As(err, &pe) {
		t.Fatalf("bad token accepted: %v", err)
	}
	if _, err := s.CreateAccount(ctx, AccountInput{Kind: "memory"}); !errors.Is(err, ErrInvalid) {
		t.Fatalf("missing token: %v", err)
	}
	a, err := s.CreateAccount(ctx, AccountInput{Kind: "memory", Credentials: map[string]string{"token": "good"}})
	if err != nil || a.Name != "Memory" || !a.Set["token"] || a.Zones != 2 {
		t.Fatalf("account = %+v, %v", a, err)
	}
	// An update with an empty secret keeps the stored one.
	if a, err = s.UpdateAccount(ctx, a.ID, AccountInput{Name: "Home", Credentials: map[string]string{}}); err != nil || a.Name != "Home" {
		t.Fatalf("update = %+v, %v", a, err)
	}
	r, err := s.CreateRecord(ctx, a.ID, "z1", provider.Record{Type: "a", Name: "www", Content: "192.0.2.1"})
	if err != nil || r.Name != "www.example.com" || r.Type != "A" {
		t.Fatalf("record = %+v, %v", r, err)
	}
	if r, _ := s.CreateRecord(ctx, a.ID, "z1", provider.Record{Type: "MX", Name: "@", Content: "mail.example.com"}); r.Name != "example.com" || r.Priority == nil {
		t.Fatalf("apex MX = %+v", r)
	}
	if _, err := s.CreateRecord(ctx, a.ID, "z1", provider.Record{Type: "PTR", Name: "x", Content: "y"}); !errors.Is(err, ErrInvalid) {
		t.Fatalf("PTR accepted: %v", err)
	}
	if _, err := s.Zone(ctx, a.ID, "nope"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("unknown zone: %v", err)
	}
}

func TestChallenges(t *testing.T) {
	ctx := context.Background()
	s := newService(t)
	if _, err := s.CreateAccount(ctx, AccountInput{Kind: "memory", Credentials: map[string]string{"token": "good"}}); err != nil {
		t.Fatal(err)
	}
	c := Challenges{Service: s}
	if !c.Covers("_acme-challenge.www.example.com") || c.Covers("_acme-challenge.example.org") {
		t.Fatal("scope does not follow the zones")
	}
	// The longest zone wins: dev.example.com, not example.com.
	name := "_acme-challenge.api.dev.example.com"
	if err := c.Present(ctx, name, "token-1"); err != nil {
		t.Fatal(err)
	}
	if err := c.Present(ctx, name, "token-1"); err != nil { // retried: no duplicate
		t.Fatal(err)
	}
	if got, _ := shared.Records(ctx, "z2", provider.Filter{Type: "TXT", Name: name}); len(got) != 1 || got[0].Content != "token-1" {
		t.Fatalf("z2 records = %+v", got)
	}
	if got, _ := shared.Records(ctx, "z1", provider.Filter{Type: "TXT", Name: name}); len(got) != 0 {
		t.Fatalf("published in the parent zone: %+v", got)
	}
	if err := c.CleanUp(ctx, name, "token-1"); err != nil {
		t.Fatal(err)
	}
	if got, _ := shared.Records(ctx, "z2", provider.Filter{Type: "TXT", Name: name}); len(got) != 0 {
		t.Fatalf("left after cleanup: %+v", got)
	}
}

func TestBuiltInProviders(t *testing.T) {
	for _, tc := range []struct {
		kind        string
		credentials map[string]string
	}{
		{"cloudflare", map[string]string{"api_token": "token"}},
		{"dnspod", map[string]string{"token": "123,secret"}},
		{"namecom", map[string]string{"user": "user", "token": "token"}},
	} {
		t.Run(tc.kind, func(t *testing.T) {
			k, ok := provider.Lookup(tc.kind)
			if !ok || len(k.Fields) == 0 {
				t.Fatal("provider not registered")
			}
			if _, err := provider.Open(tc.kind, tc.credentials); err != nil {
				t.Fatal(err)
			}
			if _, err := provider.Open(tc.kind, nil); !errors.Is(err, ErrInvalid) {
				t.Fatalf("missing credentials: %v", err)
			}
		})
	}
	if _, err := provider.Open("dnspod", map[string]string{"token": "not-a-dnspod-token"}); !errors.Is(err, ErrInvalid) {
		t.Fatalf("invalid DNSPod token: %v", err)
	}
}

func TestProviderRecordSettings(t *testing.T) {
	for _, tc := range []struct {
		kind, recordType string
		ttl, wantTTL     int
		invalid          bool
	}{
		{"cloudflare", "A", 1, 1, false},
		{"dnspod", "TXT", 1, 600, false},
		{"namecom", "TXT", 120, 300, false},
		{"namecom", "MX", 3600, 3600, false},
		{"namecom", "ANAME", 300, 300, false},
		{"namecom", "CAA", 300, 300, true},
	} {
		r := provider.Record{Type: tc.recordType, TTL: tc.ttl}
		err := checkProviderRecord(&r, ZoneView{Kind: tc.kind})
		if tc.invalid {
			if !errors.Is(err, ErrInvalid) {
				t.Fatalf("%s %s accepted", tc.kind, tc.recordType)
			}
			continue
		}
		if err != nil || r.TTL != tc.wantTTL {
			t.Fatalf("%s %s: %+v, %v", tc.kind, tc.recordType, r, err)
		}
	}
	r := provider.Record{Type: "TXT", TTL: 300}
	if err := checkProviderRecord(&r, ZoneView{Kind: "dnspod", Zone: provider.Zone{MinTTL: 300}}); err != nil || r.TTL != 300 {
		t.Fatalf("zone TTL: %+v, %v", r, err)
	}
}

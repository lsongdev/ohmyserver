// Package api manages DNS provider accounts and the zones and records they
// hold, as a JSON API with relative routes, and publishes ACME DNS-01
// records in them (see Challenges).
package api

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/lsongdev/ohmyserver/modules/domains/provider"
	_ "github.com/lsongdev/ohmyserver/modules/domains/provider/cloudflare" // registers Cloudflare
	"github.com/lsongdev/ohmyserver/modules/domains/store"
)

// ErrInvalid wraps rejected input.
var ErrInvalid = provider.ErrInvalid

// ErrNotFound reports a zone or record that does not exist.
var ErrNotFound = errors.New("not found")

func invalid(format string, args ...any) error {
	return fmt.Errorf("%w: %s", ErrInvalid, fmt.Sprintf(format, args...))
}

// zonesTTL is how long an account's zone list is reused; adding a domain
// at the provider shows up after it, or at once with a refresh.
const zonesTTL = 10 * time.Minute

// Service manages accounts and reaches their providers.
type Service struct {
	store *store.SQLite
	log   *slog.Logger

	mu    sync.Mutex
	zones map[int64]zoneCache // by account
}

type zoneCache struct {
	zones []provider.Zone
	err   error
	at    time.Time
}

// NewService returns the service.
func NewService(st *store.SQLite, log *slog.Logger) *Service {
	if log == nil {
		log = slog.Default()
	}
	return &Service{store: st, log: log, zones: map[int64]zoneCache{}}
}

// AccountView is an account as shown: which credentials are set, never
// their values.
type AccountView struct {
	store.Account
	Title string          `json:"title"` // the provider's name
	Set   map[string]bool `json:"set"`   // credential fields that have a value
	Zones int             `json:"zones"` // known zones, -1 when unknown
	Error string          `json:"error,omitempty"`
}

func (s *Service) view(a store.Account) AccountView {
	v := AccountView{Account: a, Set: map[string]bool{}, Zones: -1}
	if k, ok := provider.Lookup(a.Kind); ok {
		v.Title = k.Title
	}
	for name, value := range a.Credentials {
		v.Set[name] = value != ""
	}
	s.mu.Lock()
	if c, ok := s.zones[a.ID]; ok {
		if c.err != nil {
			v.Error = c.err.Error()
		} else {
			v.Zones = len(c.zones)
		}
	}
	s.mu.Unlock()
	return v
}

// Accounts lists the accounts.
func (s *Service) Accounts(ctx context.Context) ([]AccountView, error) {
	list, err := s.store.List(ctx)
	out := make([]AccountView, len(list))
	for i, a := range list {
		out[i] = s.view(a)
	}
	return out, err
}

// Account returns one account.
func (s *Service) Account(ctx context.Context, id int64) (AccountView, error) {
	a, err := s.store.Get(ctx, id)
	return s.view(a), err
}

// AccountInput creates or updates an account. On update, empty secret
// fields keep their stored values.
type AccountInput struct {
	Name        string            `json:"name"`
	Kind        string            `json:"kind"`
	Credentials map[string]string `json:"credentials"`
}

// merge builds the account in describes, over the stored one when updating.
func (s *Service) merge(in AccountInput, stored *store.Account) (store.Account, error) {
	a := store.Account{Name: strings.TrimSpace(in.Name), Kind: in.Kind, Credentials: map[string]string{}}
	if stored != nil {
		a.ID, a.Kind, a.CreatedAt = stored.ID, stored.Kind, stored.CreatedAt
	}
	kind, ok := provider.Lookup(a.Kind)
	if !ok {
		return a, invalid("choose a provider")
	}
	if a.Name == "" {
		a.Name = kind.Title
	}
	for _, f := range kind.Fields {
		value := strings.TrimSpace(in.Credentials[f.Name])
		if value == "" && stored != nil {
			value = stored.Credentials[f.Name]
		}
		a.Credentials[f.Name] = value
	}
	return a, nil
}

// check opens the account and lists its zones, so wrong credentials are
// refused before they are saved.
func (s *Service) check(ctx context.Context, a store.Account) ([]provider.Zone, error) {
	p, err := provider.Open(a.Kind, a.Credentials)
	if err != nil {
		return nil, err
	}
	zones, err := p.Zones(ctx)
	if err != nil {
		return nil, &ProviderError{err}
	}
	return zones, nil
}

// ProviderError is a provider refusing a request or being unreachable.
type ProviderError struct{ Err error }

func (e *ProviderError) Error() string { return e.Err.Error() }
func (e *ProviderError) Unwrap() error { return e.Err }

// Test checks credentials without saving them; id > 0 fills empty secrets
// from that account. It returns the zones the credentials reach.
func (s *Service) Test(ctx context.Context, id int64, in AccountInput) ([]provider.Zone, error) {
	var stored *store.Account
	if id > 0 {
		a, err := s.store.Get(ctx, id)
		if err != nil {
			return nil, err
		}
		stored = &a
	}
	a, err := s.merge(in, stored)
	if err != nil {
		return nil, err
	}
	return s.check(ctx, a)
}

// CreateAccount checks and saves a new account.
func (s *Service) CreateAccount(ctx context.Context, in AccountInput) (AccountView, error) {
	a, err := s.merge(in, nil)
	if err != nil {
		return AccountView{}, err
	}
	zones, err := s.check(ctx, a)
	if err != nil {
		return AccountView{}, err
	}
	if err := s.store.Create(ctx, &a); err != nil {
		return AccountView{}, err
	}
	s.remember(a.ID, zones, nil)
	return s.view(a), nil
}

// UpdateAccount checks and saves changes to an account.
func (s *Service) UpdateAccount(ctx context.Context, id int64, in AccountInput) (AccountView, error) {
	stored, err := s.store.Get(ctx, id)
	if err != nil {
		return AccountView{}, err
	}
	a, err := s.merge(in, &stored)
	if err != nil {
		return AccountView{}, err
	}
	zones, err := s.check(ctx, a)
	if err != nil {
		return AccountView{}, err
	}
	if err := s.store.Update(ctx, &a); err != nil {
		return AccountView{}, err
	}
	s.remember(a.ID, zones, nil)
	return s.view(a), nil
}

// DeleteAccount removes an account; its zones stay at the provider.
func (s *Service) DeleteAccount(ctx context.Context, id int64) error {
	s.mu.Lock()
	delete(s.zones, id)
	s.mu.Unlock()
	return s.store.Delete(ctx, id)
}

func (s *Service) remember(id int64, zones []provider.Zone, err error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.zones[id] = zoneCache{zones: zones, err: err, at: time.Now()}
}

// open returns an account's provider.
func (s *Service) open(ctx context.Context, id int64) (store.Account, provider.Provider, error) {
	a, err := s.store.Get(ctx, id)
	if err != nil {
		return a, nil, err
	}
	p, err := provider.Open(a.Kind, a.Credentials)
	return a, p, err
}

// ZoneView is a zone with the account it belongs to.
type ZoneView struct {
	provider.Zone
	Account     int64  `json:"account"`
	AccountName string `json:"account_name"`
	Kind        string `json:"kind"`
}

// Zones lists every account's zones, from the cache unless it is old or
// refresh is set. An account that cannot be reached is reported in
// Accounts and skipped here.
func (s *Service) Zones(ctx context.Context, refresh bool) ([]ZoneView, error) {
	accounts, err := s.store.List(ctx)
	if err != nil {
		return nil, err
	}
	var wg sync.WaitGroup
	for _, a := range accounts {
		s.mu.Lock()
		c, ok := s.zones[a.ID]
		s.mu.Unlock()
		if ok && !refresh && time.Since(c.at) < zonesTTL {
			continue
		}
		wg.Go(func() {
			p, err := provider.Open(a.Kind, a.Credentials)
			var zones []provider.Zone
			if err == nil {
				zones, err = p.Zones(ctx)
			}
			if err != nil {
				s.log.Warn("domains: listing zones failed", "account", a.Name, "err", err)
			}
			s.remember(a.ID, zones, err)
		})
	}
	wg.Wait()
	out := []ZoneView{}
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, a := range accounts {
		for _, z := range s.zones[a.ID].zones {
			out = append(out, ZoneView{Zone: z, Account: a.ID, AccountName: a.Name, Kind: a.Kind})
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out, nil
}

// zone finds a zone of an account by ID.
func (s *Service) zone(ctx context.Context, account int64, zoneID string) (ZoneView, error) {
	zones, err := s.Zones(ctx, false)
	if err != nil {
		return ZoneView{}, err
	}
	for _, z := range zones {
		if z.Account == account && z.ID == zoneID {
			return z, nil
		}
	}
	return ZoneView{}, fmt.Errorf("%w: no zone %s in this account", ErrNotFound, zoneID)
}

// ZoneDetail is a zone with its records.
type ZoneDetail struct {
	ZoneView
	Records []provider.Record `json:"records"`
	Proxy   bool              `json:"proxy"` // records can be proxied
}

// Zone returns a zone's records.
func (s *Service) Zone(ctx context.Context, account int64, zoneID string) (*ZoneDetail, error) {
	z, err := s.zone(ctx, account, zoneID)
	if err != nil {
		return nil, err
	}
	_, p, err := s.open(ctx, account)
	if err != nil {
		return nil, err
	}
	records, err := p.Records(ctx, zoneID, provider.Filter{})
	if err != nil {
		return nil, &ProviderError{err}
	}
	sort.SliceStable(records, func(i, j int) bool {
		if records[i].Name != records[j].Name {
			return records[i].Name < records[j].Name
		}
		return records[i].Type < records[j].Type
	})
	kind, _ := provider.Lookup(z.Kind)
	return &ZoneDetail{ZoneView: z, Records: records, Proxy: kind.Proxy}, nil
}

var recordTypes = map[string]bool{"A": true, "AAAA": true, "CNAME": true, "TXT": true, "MX": true, "NS": true, "SRV": true, "CAA": true}

// checkRecord validates r for zone and makes its name absolute: "www" and
// "@" are relative to the zone.
func checkRecord(r *provider.Record, zone string) error {
	r.Type = strings.ToUpper(strings.TrimSpace(r.Type))
	if !recordTypes[r.Type] {
		return invalid("record type %q is not supported here", r.Type)
	}
	name := provider.Fqdn(r.Name)
	switch {
	case name == "" || name == "@":
		name = zone
	case !provider.InZone(name, zone):
		name += "." + zone
	}
	r.Name = name
	r.Content = strings.TrimSpace(r.Content)
	if r.Content == "" {
		return invalid("give the record's content")
	}
	if r.TTL != 0 && r.TTL != 1 && (r.TTL < 30 || r.TTL > 86400) {
		return invalid("TTL must be automatic or between 30 and 86400 seconds")
	}
	if (r.Type == "MX" || r.Type == "SRV") && r.Priority == nil {
		p := 10
		r.Priority = &p
	}
	return nil
}

// CreateRecord adds a record to a zone.
func (s *Service) CreateRecord(ctx context.Context, account int64, zoneID string, r provider.Record) (provider.Record, error) {
	z, err := s.zone(ctx, account, zoneID)
	if err != nil {
		return r, err
	}
	if err := checkRecord(&r, z.Name); err != nil {
		return r, err
	}
	_, p, err := s.open(ctx, account)
	if err != nil {
		return r, err
	}
	created, err := p.CreateRecord(ctx, zoneID, r)
	if err != nil {
		return r, &ProviderError{err}
	}
	s.log.Info("domains: record added", "zone", z.Name, "type", created.Type, "name", created.Name)
	return created, nil
}

// UpdateRecord changes a record.
func (s *Service) UpdateRecord(ctx context.Context, account int64, zoneID, recordID string, r provider.Record) (provider.Record, error) {
	z, err := s.zone(ctx, account, zoneID)
	if err != nil {
		return r, err
	}
	if err := checkRecord(&r, z.Name); err != nil {
		return r, err
	}
	r.ID = recordID
	_, p, err := s.open(ctx, account)
	if err != nil {
		return r, err
	}
	updated, err := p.UpdateRecord(ctx, zoneID, r)
	if err != nil {
		return r, &ProviderError{err}
	}
	s.log.Info("domains: record changed", "zone", z.Name, "type", updated.Type, "name", updated.Name)
	return updated, nil
}

// DeleteRecord removes a record.
func (s *Service) DeleteRecord(ctx context.Context, account int64, zoneID, recordID string) error {
	z, err := s.zone(ctx, account, zoneID)
	if err != nil {
		return err
	}
	_, p, err := s.open(ctx, account)
	if err != nil {
		return err
	}
	if err := p.DeleteRecord(ctx, zoneID, recordID); err != nil {
		return &ProviderError{err}
	}
	s.log.Info("domains: record deleted", "zone", z.Name, "id", recordID)
	return nil
}

// Summary counts accounts and zones for an overview.
type Summary struct {
	Accounts int `json:"accounts"`
	Zones    int `json:"zones"`
}

func (s *Service) Summary(ctx context.Context) (Summary, error) {
	accounts, err := s.store.List(ctx)
	if err != nil {
		return Summary{}, err
	}
	zones, err := s.Zones(ctx, false)
	return Summary{Accounts: len(accounts), Zones: len(zones)}, err
}

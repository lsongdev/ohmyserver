package cloudflare

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"github.com/lsongdev/ohmyserver/modules/domains/provider"
)

// fake is a small Cloudflare: two pages of zones, records in memory.
func fake(t *testing.T) *Client {
	t.Helper()
	var mu sync.Mutex
	records := map[string]record{"r1": {ID: "r1", Type: "A", Name: "www.example.com", Content: "192.0.2.1", TTL: 1}}
	next := 2
	reply := func(w http.ResponseWriter, status int, result any, info any) {
		w.WriteHeader(status)
		json.NewEncoder(w).Encode(map[string]any{"success": status < 300, "result": result, "result_info": info,
			"errors": map[bool][]any{true: {}, false: {map[string]any{"code": 9109, "message": "Invalid access token"}}}[status < 300]})
	}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer good" {
			reply(w, 403, nil, nil)
			return
		}
		mu.Lock()
		defer mu.Unlock()
		switch {
		case r.URL.Path == "/zones":
			if r.URL.Query().Get("page") == "1" {
				reply(w, 200, []zone{{ID: "z1", Name: "example.com", Status: "active"}}, map[string]int{"page": 1, "total_pages": 2})
			} else {
				reply(w, 200, []zone{{ID: "z2", Name: "example.org", Status: "active"}}, map[string]int{"page": 2, "total_pages": 2})
			}
		case r.URL.Path == "/zones/z1/dns_records" && r.Method == http.MethodGet:
			var out []record
			for _, rec := range records {
				if q := r.URL.Query(); (q.Get("type") == "" || q.Get("type") == rec.Type) && (q.Get("name") == "" || q.Get("name") == rec.Name) {
					out = append(out, rec)
				}
			}
			reply(w, 200, out, map[string]int{"page": 1, "total_pages": 1})
		case r.URL.Path == "/zones/z1/dns_records" && r.Method == http.MethodPost:
			var rec record
			json.NewDecoder(r.Body).Decode(&rec)
			rec.ID = "r" + string(rune('0'+next))
			next++
			records[rec.ID] = rec
			reply(w, 200, rec, nil)
		case strings.HasPrefix(r.URL.Path, "/zones/z1/dns_records/"):
			id := strings.TrimPrefix(r.URL.Path, "/zones/z1/dns_records/")
			if r.Method == http.MethodDelete {
				delete(records, id)
				reply(w, 200, map[string]string{"id": id}, nil)
				return
			}
			var rec record
			json.NewDecoder(r.Body).Decode(&rec)
			rec.ID = id
			records[id] = rec
			reply(w, 200, rec, nil)
		default:
			reply(w, 404, nil, nil)
		}
	}))
	t.Cleanup(srv.Close)
	c := New("good")
	c.BaseURL = srv.URL
	return c
}

func TestClient(t *testing.T) {
	ctx := context.Background()
	c := fake(t)
	zones, err := c.Zones(ctx)
	if err != nil || len(zones) != 2 || zones[1].Name != "example.org" {
		t.Fatalf("zones = %+v, %v", zones, err)
	}
	created, err := c.CreateRecord(ctx, "z1", provider.Record{Type: "txt", Name: "_acme-challenge.example.com.", Content: "abc", TTL: 120})
	if err != nil || created.ID == "" || created.Type != "TXT" || created.Name != "_acme-challenge.example.com" {
		t.Fatalf("created = %+v, %v", created, err)
	}
	found, err := c.Records(ctx, "z1", provider.Filter{Type: "TXT", Name: "_acme-challenge.example.com"})
	if err != nil || len(found) != 1 || found[0].Content != "abc" {
		t.Fatalf("found = %+v, %v", found, err)
	}
	created.Content = "def"
	if updated, err := c.UpdateRecord(ctx, "z1", created); err != nil || updated.Content != "def" {
		t.Fatalf("updated = %+v, %v", updated, err)
	}
	if err := c.DeleteRecord(ctx, "z1", created.ID); err != nil {
		t.Fatal(err)
	}
	if all, _ := c.Records(ctx, "z1", provider.Filter{}); len(all) != 1 {
		t.Fatalf("records after delete = %+v", all)
	}
	bad := New("bad")
	bad.BaseURL = c.BaseURL
	var e *Error
	if _, err := bad.Zones(ctx); !errors.As(err, &e) || e.Status != 403 || !strings.Contains(err.Error(), "Invalid access token") {
		t.Fatalf("bad token: %v", err)
	}
}

func TestAuthentication(t *testing.T) {
	for _, test := range []struct {
		name        string
		credentials map[string]string
		valid       bool
	}{
		{"token", map[string]string{"api_token": "token"}, true},
		{"global key", map[string]string{"email": "user@example.com", "api_key": "key"}, true},
		{"token takes precedence", map[string]string{"api_token": "token", "email": "user@example.com", "api_key": "key"}, true},
		{"missing", nil, false},
		{"email only", map[string]string{"email": "user@example.com"}, false},
		{"key only", map[string]string{"api_key": "key"}, false},
	} {
		t.Run(test.name, func(t *testing.T) {
			p, err := provider.Open("cloudflare", test.credentials)
			if !test.valid {
				if !errors.Is(err, provider.ErrInvalid) {
					t.Fatalf("credentials accepted: %v", err)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if test.credentials["api_token"] != "" {
					if r.Header.Get("Authorization") != "Bearer token" || r.Header.Get("X-Auth-Key") != "" || r.Header.Get("X-Auth-Email") != "" {
						t.Error("incorrect token headers")
					}
				} else if r.Header.Get("Authorization") != "" || r.Header.Get("X-Auth-Email") != "user@example.com" || r.Header.Get("X-Auth-Key") != "key" {
					t.Error("incorrect global key headers")
				}
				w.Write([]byte(`{"success":true,"result":[]}`))
			}))
			defer srv.Close()
			c := p.(*Client)
			c.BaseURL = srv.URL
			if _, err := c.Zones(context.Background()); err != nil {
				t.Fatal(err)
			}
		})
	}
}

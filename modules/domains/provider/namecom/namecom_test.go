package namecom

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"

	"github.com/lsongdev/ohmyserver/modules/domains/provider"
)

func TestClient(t *testing.T) {
	records := map[int64]record{1: {ID: 1, Host: "", Type: "MX", Answer: "mail.example.com", TTL: 300, Priority: new(10)}}
	next := int64(2)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		user, token, ok := r.BasicAuth()
		if !ok || user != "user" || token != "token" {
			w.WriteHeader(401)
			w.Write([]byte(`{"message":"Unauthenticated","details":"Invalid credentials"}`))
			return
		}
		reply := func(v any) {
			t.Helper()
			if err := json.NewEncoder(w).Encode(v); err != nil {
				t.Error(err)
			}
		}
		switch {
		case r.URL.Path == "/domains":
			if r.URL.Query().Get("page") == "1" {
				reply(map[string]any{"domains": []domain{{Name: "example.com"}}, "nextPage": 2})
			} else {
				reply(map[string]any{"domains": []domain{{Name: "example.org"}}})
			}
		case r.URL.Path == "/domains/example.com/records" && r.Method == "GET":
			// A matching challenge only appears on page 2. Exact name/type filters
			// must be applied after fetching every page.
			var items []record
			for id, item := range records {
				if (r.URL.Query().Get("page") == "1") == (id == 1) {
					items = append(items, item)
				}
			}
			page := 0
			if r.URL.Query().Get("page") == "1" {
				page = 2
			}
			reply(map[string]any{"records": items, "nextPage": page})
		case r.Method == "POST" || r.Method == "PUT":
			var item record
			if err := json.NewDecoder(r.Body).Decode(&item); err != nil {
				t.Error(err)
			}
			if item.TTL < 300 {
				t.Error("invalid TTL")
			}
			if item.Host == "_acme-challenge.example.com" {
				t.Error("sent absolute host")
			}
			if r.Method == "POST" {
				item.ID = next
				next++
			} else if r.URL.Path != "/domains/example.com/records/2" || item.ID != 2 {
				t.Errorf("update ID: %s %+v", r.URL.Path, item)
			}
			records[item.ID] = item
			reply(item)
		case r.Method == "DELETE":
			delete(records, 2)
			w.WriteHeader(http.StatusNoContent)
		default:
			t.Errorf("unexpected request %s %s", r.Method, r.URL)
			w.WriteHeader(404)
		}
	}))
	defer srv.Close()
	c := New("user", "token")
	c.BaseURL = srv.URL
	ctx := context.Background()
	zones, err := c.Zones(ctx)
	if err != nil || len(zones) != 2 {
		t.Fatalf("zones: %+v, %v", zones, err)
	}
	all, err := c.Records(ctx, "example.com", provider.Filter{})
	if err != nil || len(all) != 1 || all[0].Name != "example.com" || all[0].Priority == nil || *all[0].Priority != 10 {
		t.Fatalf("records: %+v, %v", all, err)
	}
	created, err := c.CreateRecord(ctx, "example.com", provider.Record{Type: "txt", Name: "_acme-challenge.example.com.", Content: "token", TTL: 120})
	if err != nil || created.ID != "2" || created.Name != "_acme-challenge.example.com" || created.TTL != 300 {
		t.Fatalf("create: %+v, %v", created, err)
	}
	found, err := c.Records(ctx, "example.com", provider.Filter{Type: "txt", Name: "_acme-challenge.example.com."})
	if err != nil || len(found) != 1 || found[0].ID != created.ID {
		t.Fatalf("filter: %+v, %v", found, err)
	}
	created.Content = "changed"
	updated, err := c.UpdateRecord(ctx, "example.com", created)
	if err != nil || updated.Content != "changed" {
		t.Fatalf("update: %+v, %v", updated, err)
	}
	if err := c.DeleteRecord(ctx, "example.com", created.ID); err != nil {
		t.Fatal(err)
	}
	if found, err := c.Records(ctx, "example.com", provider.Filter{Type: "TXT"}); err != nil || len(found) != 0 {
		t.Fatalf("delete: %+v, %v", found, err)
	}
	if _, err := c.UpdateRecord(ctx, "example.com", provider.Record{ID: "invalid", Name: "example.com"}); !errors.Is(err, provider.ErrInvalid) {
		t.Fatalf("invalid ID: %v", err)
	}
	if _, err := c.CreateRecord(ctx, "example.com", provider.Record{Name: "other.com"}); !errors.Is(err, provider.ErrInvalid) {
		t.Fatalf("outside zone: %v", err)
	}
	c.Token = "bad"
	var apiErr *Error
	if _, err := c.Zones(ctx); !errors.As(err, &apiErr) || apiErr.Status != 401 || apiErr.Message != "Unauthenticated" {
		t.Fatalf("API error: %v", err)
	}
	cancelCtx, cancel := context.WithCancel(ctx)
	cancel()
	if _, err := c.Zones(cancelCtx); !errors.Is(err, context.Canceled) {
		t.Fatalf("cancel: %v", err)
	}
}

func TestRecordMapping(t *testing.T) {
	for _, host := range []string{"", "@", "www", "*.dev"} {
		r := record{ID: 1, Host: host, Type: "SRV", Answer: "1 5061 sip.example.com", TTL: 600, Priority: new(5)}
		common := r.toRecord("example.com")
		body, err := fromRecord("example.com", common)
		expected := host
		if expected == "@" {
			expected = ""
		}
		if err != nil || body.Host != expected || body.Answer != r.Answer || *body.Priority != 5 {
			t.Fatalf("%s: %+v, %v", strconv.Quote(host), body, err)
		}
	}
}

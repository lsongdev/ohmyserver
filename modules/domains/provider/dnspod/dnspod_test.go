package dnspod

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/lsongdev/ohmyserver/modules/domains/provider"
)

func TestClient(t *testing.T) {
	records := []record{{ID: "1", Name: "@", Type: "MX", Value: "mail.example.com", TTL: "600", MX: "10", Remark: "keep"}}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := r.ParseForm(); err != nil {
			t.Error(err)
		}
		if r.Method != "POST" || r.Form.Get("format") != "json" || r.UserAgent() == "" {
			t.Errorf("request: %+v", r)
		}
		reply := func(v map[string]any) {
			t.Helper()
			v["status"] = map[string]any{"code": "1"}
			if err := json.NewEncoder(w).Encode(v); err != nil {
				t.Error(err)
			}
		}
		if r.Form.Get("login_token") != "123,secret" {
			json.NewEncoder(w).Encode(map[string]any{"status": map[string]any{"code": "-1", "message": "Invalid token"}})
			return
		}
		switch r.URL.Path {
		case "/Domain.List":
			if r.Form.Get("offset") == "0" {
				reply(map[string]any{"domains": []any{map[string]any{"id": 1, "name": "example.com", "ttl": "600"}}, "info": map[string]any{"domain_total": "2"}})
			} else {
				reply(map[string]any{"domains": []any{map[string]any{"id": "2", "name": "example.org"}}, "info": map[string]any{"domain_total": 2}})
			}
		case "/Domain.Info":
			reply(map[string]any{"domain": map[string]any{"id": 1, "name": "example.com", "ttl": "600"}})
		case "/Record.List":
			// Simulate the service returning fewer items than the requested length.
			offset := r.Form.Get("offset")
			if offset == "0" {
				reply(map[string]any{"records": records[:1], "info": map[string]any{"record_total": len(records)}})
			} else {
				reply(map[string]any{"records": records[1:], "info": map[string]any{"record_total": len(records)}})
			}
		case "/Record.Create", "/Record.Modify":
			if r.Form.Get("sub_domain") != "_acme-challenge" || r.Form.Get("ttl") != "600" || r.Form.Get("record_type") != "TXT" {
				t.Errorf("record form: %v", r.Form)
			}
			item := record{ID: "2", Name: r.Form.Get("sub_domain"), Type: r.Form.Get("record_type"), Value: r.Form.Get("value"), TTL: number(r.Form.Get("ttl"))}
			if r.URL.Path == "/Record.Create" {
				if r.Form.Get("record_line_id") != "0" {
					t.Error("missing default line")
				}
				records = append(records, item)
			} else {
				if r.Form.Get("record_id") != "2" || r.Form.Get("record_line_id") != "10=1" || r.Form.Get("status") != "disable" || r.Form.Get("weight") != "20" {
					t.Errorf("lost provider settings: %v", r.Form)
				}
				records[1] = item
			}
			reply(map[string]any{"record": map[string]any{"id": 2}})
		case "/Record.Info":
			reply(map[string]any{"record": map[string]any{"record_line_id": "10=1", "enabled": "0", "weight": 20, "remark": "keep"}})
		case "/Record.Remove":
			if r.Form.Get("record_id") != "2" || r.Form.Get("domain_id") != "1" {
				t.Errorf("delete: %v", r.Form)
			}
			records = records[:1]
			reply(map[string]any{})
		default:
			t.Errorf("unexpected action %s", r.URL.Path)
			w.WriteHeader(404)
		}
	}))
	defer srv.Close()
	c := New("123,secret")
	c.BaseURL = srv.URL
	ctx := context.Background()
	zones, err := c.Zones(ctx)
	if err != nil || len(zones) != 2 || zones[0].ID != "1" {
		t.Fatalf("zones: %+v, %v", zones, err)
	}
	all, err := c.Records(ctx, "1", provider.Filter{})
	if err != nil || len(all) != 1 || all[0].Name != "example.com" || *all[0].Priority != 10 || all[0].Comment != "keep" {
		t.Fatalf("records: %+v, %v", all, err)
	}
	r, err := c.CreateRecord(ctx, "1", provider.Record{Type: "txt", Name: "_acme-challenge.example.com.", Content: "token", TTL: 120})
	if err != nil || r.ID != "2" || r.Name != "_acme-challenge.example.com" || r.TTL != 600 {
		t.Fatalf("create: %+v, %v", r, err)
	}
	found, err := c.Records(ctx, "1", provider.Filter{Type: "txt", Name: "_acme-challenge.example.com."})
	if err != nil || len(found) != 1 || found[0].ID != r.ID {
		t.Fatalf("filter: %+v, %v", found, err)
	}
	r.Content = "changed"
	updated, err := c.UpdateRecord(ctx, "1", r)
	if err != nil || updated.Content != "changed" || updated.Comment != "keep" {
		t.Fatalf("update: %+v, %v", updated, err)
	}
	if err := c.DeleteRecord(ctx, "1", r.ID); err != nil {
		t.Fatal(err)
	}
	if found, err := c.Records(ctx, "1", provider.Filter{Type: "TXT"}); err != nil || len(found) != 0 {
		t.Fatalf("delete: %+v, %v", found, err)
	}
	c.Token = "bad"
	var apiErr *Error
	if _, err := c.Zones(ctx); !errors.As(err, &apiErr) || apiErr.Code != "-1" {
		t.Fatalf("error: %v", err)
	}
	cancelCtx, cancel := context.WithCancel(ctx)
	cancel()
	if _, err := c.Zones(cancelCtx); !errors.Is(err, context.Canceled) {
		t.Fatalf("cancel: %v", err)
	}
}

func TestEmptyLists(t *testing.T) {
	for _, code := range []string{"9", "1"} {
		t.Run(code, func(t *testing.T) {
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Write([]byte(`{"status":{"code":"` + code + `"},"domains":[]}`))
			}))
			defer srv.Close()
			c := New("123,secret")
			c.BaseURL = srv.URL
			zones, err := c.Zones(context.Background())
			if err != nil || zones == nil || len(zones) != 0 {
				t.Fatalf("empty zones: %+v, %v", zones, err)
			}
		})
	}
}

func TestRecordMapping(t *testing.T) {
	for _, item := range []record{
		{Name: "@", Type: "MX", Value: "mail.example.com", TTL: "600", MX: "10"},
		{Name: "_sip._tcp", Type: "SRV", Value: "5 10 5060 sip.example.com.", TTL: "600"},
		{Name: "*.dev", Type: "TXT", Value: "text", TTL: "600"},
	} {
		r := item.toRecord("example.com")
		form, _, err := recordForm("1", domain{Name: "example.com", TTL: "600"}, r)
		if err != nil || form.Get("sub_domain") != item.Name || form.Get("value") != item.Value {
			t.Fatalf("round trip: %v, %v", form, err)
		}
	}
	if _, _, err := recordForm("1", domain{Name: "example.com"}, provider.Record{Name: "other.com"}); !errors.Is(err, provider.ErrInvalid) {
		t.Fatalf("outside zone: %v", err)
	}
}

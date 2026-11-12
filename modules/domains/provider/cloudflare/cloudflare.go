// Package cloudflare is the Cloudflare DNS provider, on the v4 REST API
// with an API token.
package cloudflare

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"maps"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/lsongdev/ohmyserver/modules/domains/provider"
)

func init() {
	provider.Register(provider.Kind{
		ID: "cloudflare", Title: "Cloudflare", Proxy: true, Comments: true,
		RecordTypes: []string{"A", "AAAA", "CNAME", "TXT", "MX", "NS", "SRV", "CAA"},
		Fields: []provider.Field{
			{Name: "api_token", Label: "API token", Secret: true, Optional: true,
				Hint: "With Zone › Zone › Read and Zone › DNS › Edit permissions"},
			{Name: "email", Label: "Account email", Optional: true, Hint: "Only needed with a Global API Key"},
			{Name: "api_key", Label: "Global API Key", Secret: true, Optional: true, Hint: "Only needed when no API token is set"},
		},
		Help: "Use an API token from dash.cloudflare.com › My Profile › API Tokens (Edit zone DNS), or supply both your account email and Global API Key. A token takes precedence when set.",
		New: func(c map[string]string) (provider.Provider, error) {
			client := New(strings.TrimSpace(c["api_token"]))
			client.Email, client.APIKey = strings.TrimSpace(c["email"]), strings.TrimSpace(c["api_key"])
			if client.Token == "" && (client.Email == "" || client.APIKey == "") {
				return nil, fmt.Errorf("%w: supply an API token or both account email and Global API Key", provider.ErrInvalid)
			}
			return client, nil
		},
	})
}

// DefaultURL is the API's base.
const DefaultURL = "https://api.cloudflare.com/client/v4"

// Client is a Cloudflare account reached with an API token.
type Client struct {
	Token   string
	Email   string
	APIKey  string
	BaseURL string
	HTTP    *http.Client
}

// New returns a client for token.
func New(token string) *Client {
	return &Client{Token: token, BaseURL: DefaultURL, HTTP: &http.Client{Timeout: 30 * time.Second}}
}

// Error is an error the API answered with.
type Error struct {
	Status   int
	Messages []string
}

func (e *Error) Error() string {
	return fmt.Sprintf("cloudflare: %d %s", e.Status, strings.Join(e.Messages, "; "))
}

type envelope struct {
	Success bool            `json:"success"`
	Result  json.RawMessage `json:"result"`
	Errors  []struct {
		Code    int    `json:"code"`
		Message string `json:"message"`
	} `json:"errors"`
	ResultInfo *struct {
		Page       int `json:"page"`
		TotalPages int `json:"total_pages"`
	} `json:"result_info"`
}

func (c *Client) do(ctx context.Context, method, path string, query url.Values, body, out any) (*envelope, error) {
	target := c.BaseURL + path
	if len(query) > 0 {
		target += "?" + query.Encode()
	}
	var reader io.Reader
	if body != nil {
		data, err := json.Marshal(body)
		if err != nil {
			return nil, err
		}
		reader = bytes.NewReader(data)
	}
	req, err := http.NewRequestWithContext(ctx, method, target, reader)
	if err != nil {
		return nil, err
	}
	if c.Token != "" {
		req.Header.Set("Authorization", "Bearer "+c.Token)
	} else {
		req.Header.Set("X-Auth-Email", c.Email)
		req.Header.Set("X-Auth-Key", c.APIKey)
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	resp, err := c.HTTP.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	data, err := io.ReadAll(io.LimitReader(resp.Body, 16<<20))
	if err != nil {
		return nil, err
	}
	var env envelope
	if err := json.Unmarshal(data, &env); err != nil {
		return nil, fmt.Errorf("cloudflare: %s: unexpected response", resp.Status)
	}
	if !env.Success || resp.StatusCode/100 != 2 {
		e := &Error{Status: resp.StatusCode}
		for _, m := range env.Errors {
			e.Messages = append(e.Messages, fmt.Sprintf("%s (%d)", m.Message, m.Code))
		}
		return nil, e
	}
	if out != nil && len(env.Result) > 0 {
		if err := json.Unmarshal(env.Result, out); err != nil {
			return nil, err
		}
	}
	return &env, nil
}

// pages fetches every page of a list.
func pages[T any](ctx context.Context, c *Client, path string, query url.Values, perPage int) ([]T, error) {
	var all []T
	for page := 1; ; page++ {
		q := url.Values{}
		maps.Copy(q, query)
		q.Set("page", strconv.Itoa(page))
		q.Set("per_page", strconv.Itoa(perPage))
		var items []T
		env, err := c.do(ctx, http.MethodGet, path, q, nil, &items)
		if err != nil {
			return nil, err
		}
		all = append(all, items...)
		if env.ResultInfo == nil || page >= env.ResultInfo.TotalPages || len(items) == 0 {
			return all, nil
		}
	}
}

type zone struct {
	ID          string   `json:"id"`
	Name        string   `json:"name"`
	Status      string   `json:"status"`
	NameServers []string `json:"name_servers"`
}

// Zones lists the zones the token can see.
func (c *Client) Zones(ctx context.Context) ([]provider.Zone, error) {
	items, err := pages[zone](ctx, c, "/zones", nil, 50)
	if err != nil {
		return nil, err
	}
	out := make([]provider.Zone, len(items))
	for i, z := range items {
		out[i] = provider.Zone{ID: z.ID, Name: z.Name, Status: z.Status, NameServers: z.NameServers}
	}
	return out, nil
}

type record struct {
	ID       string `json:"id,omitempty"`
	Type     string `json:"type"`
	Name     string `json:"name"`
	Content  string `json:"content"`
	TTL      int    `json:"ttl"`
	Priority *int   `json:"priority,omitempty"`
	Proxied  *bool  `json:"proxied,omitempty"`
	Comment  string `json:"comment,omitempty"`
}

func (r record) toRecord() provider.Record {
	return provider.Record{ID: r.ID, Type: r.Type, Name: r.Name, Content: r.Content, TTL: r.TTL,
		Priority: r.Priority, Proxied: r.Proxied, Comment: r.Comment}
}

func fromRecord(r provider.Record) record {
	ttl := r.TTL
	if ttl == 0 {
		ttl = 1 // automatic
	}
	return record{Type: strings.ToUpper(r.Type), Name: provider.Fqdn(r.Name), Content: r.Content, TTL: ttl,
		Priority: r.Priority, Proxied: r.Proxied, Comment: r.Comment}
}

func recordsPath(zoneID string) string { return "/zones/" + url.PathEscape(zoneID) + "/dns_records" }

// Records lists a zone's records.
func (c *Client) Records(ctx context.Context, zoneID string, f provider.Filter) ([]provider.Record, error) {
	q := url.Values{}
	if f.Type != "" {
		q.Set("type", strings.ToUpper(f.Type))
	}
	if f.Name != "" {
		q.Set("name", provider.Fqdn(f.Name))
	}
	items, err := pages[record](ctx, c, recordsPath(zoneID), q, 100)
	if err != nil {
		return nil, err
	}
	out := make([]provider.Record, len(items))
	for i, r := range items {
		out[i] = r.toRecord()
	}
	return out, nil
}

func (c *Client) CreateRecord(ctx context.Context, zoneID string, r provider.Record) (provider.Record, error) {
	var created record
	_, err := c.do(ctx, http.MethodPost, recordsPath(zoneID), nil, fromRecord(r), &created)
	return created.toRecord(), err
}

func (c *Client) UpdateRecord(ctx context.Context, zoneID string, r provider.Record) (provider.Record, error) {
	if r.ID == "" {
		return provider.Record{}, fmt.Errorf("%w: the record has no ID", provider.ErrInvalid)
	}
	var updated record
	_, err := c.do(ctx, http.MethodPut, recordsPath(zoneID)+"/"+url.PathEscape(r.ID), nil, fromRecord(r), &updated)
	return updated.toRecord(), err
}

func (c *Client) DeleteRecord(ctx context.Context, zoneID, recordID string) error {
	_, err := c.do(ctx, http.MethodDelete, recordsPath(zoneID)+"/"+url.PathEscape(recordID), nil, nil, nil)
	return err
}

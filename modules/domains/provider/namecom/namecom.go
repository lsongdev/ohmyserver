// Package namecom manages Name.com DNS through its v4 REST API.
package namecom

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/lsongdev/ohmyserver/modules/domains/provider"
)

func init() {
	provider.Register(provider.Kind{
		ID: "namecom", Title: "Name.com", MinTTL: 300,
		RecordTypes: []string{"A", "AAAA", "ANAME", "CNAME", "TXT", "MX", "NS", "SRV"},
		Fields: []provider.Field{
			{Name: "user", Label: "Username"},
			{Name: "token", Label: "API token", Secret: true},
		},
		Help: "Enable API access and create a production API token at name.com/account/settings/api. Use your Name.com username, not your email address.",
		New: func(c map[string]string) (provider.Provider, error) {
			return New(strings.TrimSpace(c["user"]), strings.TrimSpace(c["token"])), nil
		},
	})
}

const DefaultURL = "https://api.name.com/v4"

type Client struct {
	User, Token string
	BaseURL     string
	HTTP        *http.Client
}

func New(user, token string) *Client {
	return &Client{User: user, Token: token, BaseURL: DefaultURL, HTTP: &http.Client{Timeout: 30 * time.Second}}
}

type Error struct {
	Status           int
	Message, Details string
}

func (e *Error) Error() string {
	return fmt.Sprintf("name.com: %d %s %s", e.Status, e.Message, e.Details)
}

func (c *Client) do(ctx context.Context, method, path string, body, out any) error {
	var reader io.Reader
	if body != nil {
		data, err := json.Marshal(body)
		if err != nil {
			return err
		}
		reader = bytes.NewReader(data)
	}
	req, err := http.NewRequestWithContext(ctx, method, c.BaseURL+path, reader)
	if err != nil {
		return err
	}
	req.SetBasicAuth(c.User, c.Token)
	req.Header.Set("Accept", "application/json")
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	resp, err := c.HTTP.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	data, err := io.ReadAll(io.LimitReader(resp.Body, 16<<20))
	if err != nil {
		return err
	}
	if resp.StatusCode/100 != 2 {
		var detail struct {
			Message string `json:"message"`
			Details string `json:"details"`
		}
		_ = json.Unmarshal(data, &detail)
		return &Error{Status: resp.StatusCode, Message: detail.Message, Details: detail.Details}
	}
	if out != nil {
		if err := json.Unmarshal(data, out); err != nil {
			return fmt.Errorf("name.com: %s: unexpected response", resp.Status)
		}
	}
	return nil
}

type domain struct {
	Name        string   `json:"domainName"`
	NameServers []string `json:"nameservers"`
}

func (c *Client) Zones(ctx context.Context) ([]provider.Zone, error) {
	out := []provider.Zone{}
	for page := 1; ; {
		var result struct {
			Domains  []domain `json:"domains"`
			NextPage int      `json:"nextPage"`
		}
		if err := c.do(ctx, http.MethodGet, "/domains?perPage=100&page="+strconv.Itoa(page), nil, &result); err != nil {
			return nil, err
		}
		for _, z := range result.Domains {
			out = append(out, provider.Zone{ID: provider.Fqdn(z.Name), Name: provider.Fqdn(z.Name), NameServers: z.NameServers})
		}
		if result.NextPage == 0 {
			return out, nil
		}
		if result.NextPage <= page {
			return nil, fmt.Errorf("name.com: invalid next page %d", result.NextPage)
		}
		page = result.NextPage
	}
}

type record struct {
	ID       int64  `json:"id,omitempty"`
	Host     string `json:"host"`
	Type     string `json:"type"`
	Answer   string `json:"answer"`
	TTL      int    `json:"ttl"`
	Priority *int   `json:"priority,omitempty"`
}

func (r record) toRecord(zone string) provider.Record {
	name := provider.Fqdn(zone)
	if r.Host != "" && r.Host != "@" {
		name = provider.Fqdn(r.Host) + "." + name
	}
	return provider.Record{ID: strconv.FormatInt(r.ID, 10), Name: name, Type: r.Type, Content: r.Answer, TTL: r.TTL, Priority: r.Priority}
}

func recordsPath(zone string) string { return "/domains/" + url.PathEscape(zone) + "/records" }

func (c *Client) Records(ctx context.Context, zone string, f provider.Filter) ([]provider.Record, error) {
	out := []provider.Record{}
	for page := 1; ; {
		var result struct {
			Records  []record `json:"records"`
			NextPage int      `json:"nextPage"`
		}
		if err := c.do(ctx, http.MethodGet, recordsPath(zone)+"?perPage=100&page="+strconv.Itoa(page), nil, &result); err != nil {
			return nil, err
		}
		for _, item := range result.Records {
			r := item.toRecord(zone)
			if (f.Type == "" || r.Type == strings.ToUpper(f.Type)) && (f.Name == "" || r.Name == provider.Fqdn(f.Name)) {
				out = append(out, r)
			}
		}
		if result.NextPage == 0 {
			return out, nil
		}
		if result.NextPage <= page {
			return nil, fmt.Errorf("name.com: invalid next page %d", result.NextPage)
		}
		page = result.NextPage
	}
}

func fromRecord(zone string, r provider.Record) (record, error) {
	name, zone := provider.Fqdn(r.Name), provider.Fqdn(zone)
	if !provider.InZone(name, zone) {
		return record{}, fmt.Errorf("%w: record name is outside %s", provider.ErrInvalid, zone)
	}
	host := ""
	if name != zone {
		host = strings.TrimSuffix(name, "."+zone)
	}
	ttl := max(r.TTL, 300)
	return record{Host: host, Type: strings.ToUpper(r.Type), Answer: r.Content, TTL: ttl, Priority: r.Priority}, nil
}

func (c *Client) CreateRecord(ctx context.Context, zone string, r provider.Record) (provider.Record, error) {
	body, err := fromRecord(zone, r)
	if err != nil {
		return provider.Record{}, err
	}
	var result record
	err = c.do(ctx, http.MethodPost, recordsPath(zone), body, &result)
	return result.toRecord(zone), err
}

func recordID(id string) (int64, error) {
	n, err := strconv.ParseInt(id, 10, 64)
	if err != nil || n <= 0 {
		return 0, fmt.Errorf("%w: invalid record ID", provider.ErrInvalid)
	}
	return n, nil
}

func (c *Client) UpdateRecord(ctx context.Context, zone string, r provider.Record) (provider.Record, error) {
	body, err := fromRecord(zone, r)
	if err != nil {
		return provider.Record{}, err
	}
	body.ID, err = recordID(r.ID)
	if err != nil {
		return provider.Record{}, err
	}
	var result record
	err = c.do(ctx, http.MethodPut, recordsPath(zone)+"/"+strconv.FormatInt(body.ID, 10), body, &result)
	return result.toRecord(zone), err
}

func (c *Client) DeleteRecord(ctx context.Context, zone, id string) error {
	n, err := recordID(id)
	if err != nil {
		return err
	}
	return c.do(ctx, http.MethodDelete, recordsPath(zone)+"/"+strconv.FormatInt(n, 10), nil, nil)
}

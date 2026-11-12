// Package dnspod implements DNSPod's traditional mainland API (DNSPod Token).
package dnspod

import (
	"context"
	"encoding/json"
	"errors"
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
		ID: "dnspod", Title: "DNSPod", MinTTL: 600,
		RecordTypes: []string{"A", "AAAA", "CNAME", "TXT", "MX", "NS", "SRV", "CAA"},
		Fields:      []provider.Field{{Name: "token", Label: "DNSPod Token", Secret: true, Hint: "Token ID and token joined with a comma: ID,Token"}},
		Help:        "Create a DNSPod Token in console.dnspod.cn › API tokens. Use ID,Token from a main account; Tencent Cloud SecretId/SecretKey credentials are not supported by this API.",
		New: func(c map[string]string) (provider.Provider, error) {
			token := strings.TrimSpace(c["token"])
			id, secret, ok := strings.Cut(token, ",")
			if !ok || strings.TrimSpace(id) == "" || strings.TrimSpace(secret) == "" {
				return nil, fmt.Errorf("%w: DNSPod Token must be ID,Token", provider.ErrInvalid)
			}
			return New(strings.TrimSpace(id) + "," + strings.TrimSpace(secret)), nil
		},
	})
}

const DefaultURL = "https://dnsapi.cn"

type Client struct {
	Token, BaseURL string
	HTTP           *http.Client
}

func New(token string) *Client {
	return &Client{Token: token, BaseURL: DefaultURL, HTTP: &http.Client{Timeout: 30 * time.Second}}
}

// DNSPod mixes JSON numbers and strings for IDs, counts and TTLs.
type number string

func (n *number) UnmarshalJSON(data []byte) error {
	if len(data) > 0 && data[0] == '"' {
		var s string
		if err := json.Unmarshal(data, &s); err != nil {
			return err
		}
		*n = number(s)
	} else if string(data) != "null" {
		*n = number(data)
	}
	return nil
}
func (n number) int() int { i, _ := strconv.Atoi(string(n)); return i }

type Error struct {
	Status        int
	Code, Message string
}

func (e *Error) Error() string { return fmt.Sprintf("dnspod: %d %s (%s)", e.Status, e.Message, e.Code) }

func (c *Client) do(ctx context.Context, action string, form url.Values, out any) error {
	if form == nil {
		form = url.Values{}
	}
	form.Set("login_token", c.Token)
	form.Set("format", "json")
	form.Set("error_on_empty", "no")
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.BaseURL+"/"+action, strings.NewReader(form.Encode()))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("User-Agent", "OhMyServer DNS provider (+https://github.com/lsongdev/ohmyserver)")
	resp, err := c.HTTP.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	data, err := io.ReadAll(io.LimitReader(resp.Body, 16<<20))
	if err != nil {
		return err
	}
	var env struct {
		Status struct {
			Code    number `json:"code"`
			Message string `json:"message"`
		} `json:"status"`
	}
	if err := json.Unmarshal(data, &env); err != nil {
		return fmt.Errorf("dnspod: %s: unexpected response", resp.Status)
	}
	if resp.StatusCode/100 != 2 || env.Status.Code != "1" {
		return &Error{Status: resp.StatusCode, Code: string(env.Status.Code), Message: env.Status.Message}
	}
	if out != nil {
		return json.Unmarshal(data, out)
	}
	return nil
}

type domain struct {
	ID          number   `json:"id"`
	Name        string   `json:"name"`
	Punycode    string   `json:"punycode"`
	Status      string   `json:"status"`
	TTL         number   `json:"ttl"`
	NameServers []string `json:"grade_ns"`
}

func (d domain) name() string {
	if d.Punycode != "" {
		return provider.Fqdn(d.Punycode)
	}
	return provider.Fqdn(d.Name)
}

func (c *Client) Zones(ctx context.Context) ([]provider.Zone, error) {
	out := []provider.Zone{}
	for offset := 0; ; {
		var result struct {
			Domains []domain `json:"domains"`
			Info    struct {
				Total number `json:"domain_total"`
			} `json:"info"`
		}
		err := c.do(ctx, "Domain.List", url.Values{"type": {"all"}, "offset": {strconv.Itoa(offset)}, "length": {"100"}}, &result)
		var apiErr *Error
		if errors.As(err, &apiErr) && apiErr.Status/100 == 2 && apiErr.Code == "9" {
			return out, nil
		}
		if err != nil {
			return nil, err
		}
		for _, z := range result.Domains {
			minTTL := z.TTL.int()
			if minTTL <= 1 {
				minTTL = 600
			}
			out = append(out, provider.Zone{ID: string(z.ID), Name: z.name(), Status: z.Status, NameServers: z.NameServers, MinTTL: minTTL})
		}
		offset += len(result.Domains)
		if len(result.Domains) == 0 || offset >= result.Info.Total.int() {
			return out, nil
		}
	}
}

func (c *Client) domain(ctx context.Context, id string) (domain, error) {
	var result struct {
		Domain domain `json:"domain"`
	}
	err := c.do(ctx, "Domain.Info", url.Values{"domain_id": {id}}, &result)
	if err == nil && result.Domain.Name == "" {
		err = fmt.Errorf("dnspod: missing domain name")
	}
	return result.Domain, err
}

type record struct {
	ID     number `json:"id"`
	Name   string `json:"name"`
	Type   string `json:"type"`
	Value  string `json:"value"`
	TTL    number `json:"ttl"`
	MX     number `json:"mx"`
	Remark string `json:"remark"`
}

func (r record) toRecord(zone string) provider.Record {
	name := provider.Fqdn(zone)
	if r.Name != "" && r.Name != "@" {
		name = provider.Fqdn(r.Name) + "." + name
	}
	out := provider.Record{ID: string(r.ID), Name: name, Type: r.Type, Content: r.Value, TTL: r.TTL.int(), Comment: r.Remark}
	if r.Type == "MX" {
		p := r.MX.int()
		out.Priority = &p
	}
	if r.Type == "SRV" {
		if p, rest, ok := strings.Cut(r.Value, " "); ok {
			if priority, err := strconv.Atoi(p); err == nil {
				out.Priority, out.Content = &priority, rest
			}
		}
	}
	return out
}

func (c *Client) Records(ctx context.Context, id string, f provider.Filter) ([]provider.Record, error) {
	z, err := c.domain(ctx, id)
	if err != nil {
		return nil, err
	}
	out := []provider.Record{}
	for offset := 0; ; {
		var result struct {
			Records []record `json:"records"`
			Info    struct {
				Total number `json:"record_total"`
			} `json:"info"`
		}
		// Filter locally: DNSPod's sub_domain filter need not be an exact match.
		err := c.do(ctx, "Record.List", url.Values{"domain_id": {id}, "offset": {strconv.Itoa(offset)}, "length": {"100"}}, &result)
		var apiErr *Error
		if errors.As(err, &apiErr) && apiErr.Status/100 == 2 && apiErr.Code == "10" {
			return out, nil
		}
		if err != nil {
			return nil, err
		}
		for _, item := range result.Records {
			r := item.toRecord(z.name())
			if (f.Type == "" || r.Type == strings.ToUpper(f.Type)) && (f.Name == "" || r.Name == provider.Fqdn(f.Name)) {
				out = append(out, r)
			}
		}
		offset += len(result.Records)
		if len(result.Records) == 0 || offset >= result.Info.Total.int() {
			return out, nil
		}
	}
}

func recordForm(id string, z domain, r provider.Record) (url.Values, provider.Record, error) {
	name, zone := provider.Fqdn(r.Name), z.name()
	if !provider.InZone(name, zone) {
		return nil, r, fmt.Errorf("%w: record name is outside %s", provider.ErrInvalid, zone)
	}
	host := "@"
	if name != zone {
		host = strings.TrimSuffix(name, "."+zone)
	}
	r.Name, r.Type = name, strings.ToUpper(r.Type)
	minTTL := z.TTL.int()
	if minTTL <= 1 {
		minTTL = 600
	}
	r.TTL = max(r.TTL, minTTL)
	value := r.Content
	if r.Type == "SRV" && r.Priority != nil {
		value = strconv.Itoa(*r.Priority) + " " + value
	}
	form := url.Values{"domain_id": {id}, "sub_domain": {host}, "record_type": {r.Type}, "record_line_id": {"0"}, "value": {value}, "ttl": {strconv.Itoa(r.TTL)}}
	if r.Type == "MX" && r.Priority != nil {
		form.Set("mx", strconv.Itoa(*r.Priority))
	}
	// Remarks use a separate endpoint; keep existing remarks untouched.
	r.Comment = ""
	return form, r, nil
}

func (c *Client) CreateRecord(ctx context.Context, id string, r provider.Record) (provider.Record, error) {
	z, err := c.domain(ctx, id)
	if err != nil {
		return r, err
	}
	form, r, err := recordForm(id, z, r)
	if err != nil {
		return r, err
	}
	var result struct {
		Record struct {
			ID number `json:"id"`
		} `json:"record"`
	}
	err = c.do(ctx, "Record.Create", form, &result)
	r.ID = string(result.Record.ID)
	if err == nil && r.ID == "" {
		err = fmt.Errorf("dnspod: missing created record ID")
	}
	return r, err
}

func (c *Client) UpdateRecord(ctx context.Context, id string, r provider.Record) (provider.Record, error) {
	if r.ID == "" {
		return r, fmt.Errorf("%w: the record has no ID", provider.ErrInvalid)
	}
	z, err := c.domain(ctx, id)
	if err != nil {
		return r, err
	}
	form, r, err := recordForm(id, z, r)
	if err != nil {
		return r, err
	}
	form.Set("record_id", r.ID)
	// Record.Modify defaults to the default line and enabled state. Preserve
	// these vendor settings even though the common record editor has no fields for them.
	var existing struct {
		Record struct {
			LineID  string `json:"record_line_id"`
			Line    string `json:"record_line"`
			Enabled number `json:"enabled"`
			Weight  number `json:"weight"`
			Remark  string `json:"remark"`
		} `json:"record"`
	}
	if err := c.do(ctx, "Record.Info", url.Values{"domain_id": {id}, "record_id": {r.ID}}, &existing); err != nil {
		return r, err
	}
	if existing.Record.LineID != "" {
		form.Set("record_line_id", existing.Record.LineID)
	} else if existing.Record.Line != "" {
		form.Del("record_line_id")
		form.Set("record_line", existing.Record.Line)
	}
	if existing.Record.Enabled == "0" {
		form.Set("status", "disable")
	}
	if existing.Record.Weight != "" {
		form.Set("weight", string(existing.Record.Weight))
	}
	r.Comment = existing.Record.Remark
	err = c.do(ctx, "Record.Modify", form, nil)
	return r, err
}

func (c *Client) DeleteRecord(ctx context.Context, id, recordID string) error {
	if recordID == "" {
		return fmt.Errorf("%w: the record has no ID", provider.ErrInvalid)
	}
	return c.do(ctx, "Record.Remove", url.Values{"domain_id": {id}, "record_id": {recordID}}, nil)
}

// Package clash provides a Go client for a Clash-compatible controller API.
package clash

import (
	"context"

	"github.com/lsongdev/clashapi-go/clashapi"
)

// Client wraps the controller API and keeps its transport implementation
// private to this module.
type Client struct{ api *clashapi.Client }

// NewClient creates a controller client. apiURL is the controller base URL;
// secret is sent as its bearer token when non-empty.
func NewClient(apiURL, secret string) *Client {
	return &Client{api: clashapi.NewClient(apiURL, secret)}
}

type (
	Config              = clashapi.Config
	Proxy               = clashapi.Proxy
	ProxiesResponse     = clashapi.ProxiesResponse
	Rule                = clashapi.Rule
	Provider            = clashapi.Provider
	RuleProvider        = clashapi.RuleProvider
	ConnectionsResponse = clashapi.ConnectionsResponse
	Traffic             = clashapi.Traffic
)

func (c *Client) TestConnection(ctx context.Context) error       { return c.api.TestConnection(ctx) }
func (c *Client) GetConfig(ctx context.Context) (*Config, error) { return c.api.GetConfig(ctx) }
func (c *Client) UpdateConfig(ctx context.Context, patch map[string]any) error {
	return c.api.UpdateConfig(ctx, patch)
}
func (c *Client) SetMode(ctx context.Context, mode string) error { return c.api.SetMode(ctx, mode) }
func (c *Client) GetProxies(ctx context.Context) (*ProxiesResponse, error) {
	return c.api.GetProxies(ctx)
}
func (c *Client) GetProxyProviders(ctx context.Context) ([]Provider, error) {
	return c.api.GetProxyProviders(ctx)
}
func (c *Client) GetRules(ctx context.Context) ([]Rule, error) { return c.api.GetRules(ctx) }
func (c *Client) GetRuleProviders(ctx context.Context) ([]RuleProvider, error) {
	return c.api.GetRuleProviders(ctx)
}
func (c *Client) GetConnections(ctx context.Context) (*ConnectionsResponse, error) {
	return c.api.GetConnections(ctx)
}
func (c *Client) SelectProxy(ctx context.Context, group, name string) error {
	return c.api.SelectProxy(ctx, group, name)
}
func (c *Client) GetProxyDelay(ctx context.Context, name, testURL string, timeout int) (int, error) {
	return c.api.GetProxyDelay(ctx, name, testURL, timeout)
}
func (c *Client) CloseConnection(ctx context.Context, id string) error {
	return c.api.CloseConnection(ctx, id)
}
func (c *Client) GetTrafficStream(ctx context.Context) (<-chan Traffic, error) {
	return c.api.GetTrafficStream(ctx)
}
func (c *Client) GetConnectionsStream(ctx context.Context) (<-chan ConnectionsResponse, error) {
	return c.api.GetConnectionsStream(ctx)
}

package web

import (
	"context"
	"fmt"
	"net/url"
	"time"

	clashclient "github.com/lsongdev/ohmyserver/modules/clash/clash"
)

// Manager is the standalone implementation of the module's API service.
// Hosts may instead provide their own Service implementation.
type Manager struct{ repository *Repository }

func NewManager(repository *Repository) *Manager { return &Manager{repository: repository} }

func (m *Manager) ListClash(ctx context.Context) ([]DTO, error) {
	items, err := m.repository.List(ctx)
	if err != nil {
		return nil, err
	}
	result := make([]DTO, 0, len(items))
	for i := range items {
		result = append(result, dtoFromServer(&items[i]))
	}
	return result, nil
}

func (m *Manager) CreateClash(ctx context.Context, input Input) (DTO, error) {
	secret := ""
	if input.Secret != nil {
		secret = *input.Secret
	}
	item := &Server{Name: input.Name, APIURL: input.APIURL, Secret: secret, Description: input.Description}
	if err := m.repository.Create(ctx, item); err != nil {
		return DTO{}, err
	}
	return dtoFromServer(item), nil
}

func (m *Manager) GetClash(ctx context.Context, id int64) (DTO, error) {
	item, err := m.repository.Get(ctx, id)
	if err != nil {
		return DTO{}, err
	}
	return dtoFromServer(item), nil
}

func (m *Manager) UpdateClash(ctx context.Context, id int64, input Input) (DTO, error) {
	item, err := m.repository.Get(ctx, id)
	if err != nil {
		return DTO{}, err
	}
	item.Name, item.APIURL, item.Description = input.Name, input.APIURL, input.Description
	if input.Secret != nil {
		item.Secret = *input.Secret
	}
	if err := m.repository.Update(ctx, item); err != nil {
		return DTO{}, err
	}
	return dtoFromServer(item), nil
}

func (m *Manager) DeleteClash(ctx context.Context, id int64) error {
	return m.repository.Delete(ctx, id)
}

func (m *Manager) TestClash(ctx context.Context, id int64) error {
	client, callCtx, cancel, err := m.client(ctx, id)
	if err != nil {
		return err
	}
	defer cancel()
	return client.TestConnection(callCtx)
}

func (m *Manager) ClashProxies(ctx context.Context, id int64) (any, error) {
	client, callCtx, cancel, err := m.client(ctx, id)
	if err != nil {
		return nil, err
	}
	defer cancel()
	response, err := client.GetProxies(callCtx)
	if err != nil {
		return nil, err
	}
	providers, _ := client.GetProxyProviders(callCtx)
	groups := make(map[string]clashclient.Proxy)
	for name, proxy := range response.Proxies {
		if len(proxy.All) == 0 {
			continue
		}
		group := *proxy
		group.All = nil
		group.Proxies = make([]clashclient.Proxy, 0, len(proxy.All))
		for _, member := range proxy.All {
			if item, ok := response.Proxies[member]; ok {
				group.Proxies = append(group.Proxies, *item)
			}
		}
		groups[name] = group
	}
	return map[string]any{"proxies": groups, "providers": providers}, nil
}

func (m *Manager) ClashRules(ctx context.Context, id int64) (any, error) {
	client, callCtx, cancel, err := m.client(ctx, id)
	if err != nil {
		return nil, err
	}
	defer cancel()
	rules, err := client.GetRules(callCtx)
	if err != nil {
		return nil, err
	}
	providers, _ := client.GetRuleProviders(callCtx)
	return map[string]any{"rules": rules, "providers": providers}, nil
}

func (m *Manager) ClashConnections(ctx context.Context, id int64) (any, error) {
	client, callCtx, cancel, err := m.client(ctx, id)
	if err != nil {
		return nil, err
	}
	defer cancel()
	return client.GetConnections(callCtx)
}

func (m *Manager) ClashConfig(ctx context.Context, id int64) (any, error) {
	client, callCtx, cancel, err := m.client(ctx, id)
	if err != nil {
		return nil, err
	}
	defer cancel()
	config, err := client.GetConfig(callCtx)
	if err != nil {
		return nil, err
	}
	return configDTO(config), nil
}

func (m *Manager) UpdateClashConfig(ctx context.Context, id int64, patch map[string]any) (any, error) {
	allowed := map[string]bool{"mode": true, "mixed-port": true, "port": true, "socks-port": true, "redir-port": true, "log-level": true, "allow-lan": true, "ipv6": true}
	if len(patch) == 0 {
		return nil, fmt.Errorf("configuration patch is empty")
	}
	for key := range patch {
		if !allowed[key] {
			return nil, fmt.Errorf("configuration field %q is not editable", key)
		}
	}
	client, callCtx, cancel, err := m.client(ctx, id)
	if err != nil {
		return nil, err
	}
	defer cancel()
	if err := client.UpdateConfig(callCtx, patch); err != nil {
		return nil, err
	}
	config, err := client.GetConfig(callCtx)
	if err != nil {
		return nil, err
	}
	return configDTO(config), nil
}

func (m *Manager) ClashAction(ctx context.Context, id int64, action Action) (any, error) {
	client, callCtx, cancel, err := m.client(ctx, id)
	if err != nil {
		return nil, err
	}
	defer cancel()
	switch action.Action {
	case "select":
		if action.Selector == "" || action.ProxyName == "" {
			return nil, fmt.Errorf("selector and proxy_name are required")
		}
		err = client.SelectProxy(callCtx, url.PathEscape(action.Selector), action.ProxyName)
	case "delay":
		if action.ProxyName == "" {
			return nil, fmt.Errorf("proxy_name is required")
		}
		var delay int
		delay, err = client.GetProxyDelay(callCtx, url.PathEscape(action.ProxyName), "", 5000)
		if err == nil {
			return map[string]int{"delay": delay}, nil
		}
	case "mode":
		if action.Mode == "" {
			return nil, fmt.Errorf("mode is required")
		}
		err = client.SetMode(callCtx, action.Mode)
	case "close":
		if action.ID == "" {
			return nil, fmt.Errorf("connection id is required")
		}
		err = client.CloseConnection(callCtx, url.PathEscape(action.ID))
	default:
		return nil, fmt.Errorf("unsupported action %q", action.Action)
	}
	if err != nil {
		return nil, err
	}
	return map[string]string{"status": "ok"}, nil
}

func (m *Manager) OpenClashStream(ctx context.Context, id int64) (Stream, error) {
	item, err := m.repository.Get(ctx, id)
	if err != nil {
		return nil, err
	}
	client := clashclient.NewClient(item.APIURL, item.Secret)
	// Keep the client request scope alive until the returned stream is closed.
	streamCtx, streamCancel := context.WithCancel(ctx)
	traffic, trafficErr := client.GetTrafficStream(streamCtx)
	connections, connectionErr := client.GetConnectionsStream(streamCtx)
	if connectionErr != nil {
		streamCancel()
		return nil, connectionErr
	}
	if trafficErr != nil {
		traffic = nil
	}
	return &clashStream{cancel: streamCancel, traffic: traffic, connections: connections, trafficOpen: traffic != nil, connectionsOpen: true}, nil
}

func (m *Manager) client(ctx context.Context, id int64) (*clashclient.Client, context.Context, context.CancelFunc, error) {
	item, err := m.repository.Get(ctx, id)
	if err != nil {
		return nil, nil, nil, err
	}
	callCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	return clashclient.NewClient(item.APIURL, item.Secret), callCtx, cancel, nil
}

func dtoFromServer(item *Server) DTO {
	return DTO{ID: item.ID, Name: item.Name, APIURL: item.APIURL, Status: item.Status, Description: item.Description, HasSecret: item.Secret != "", CreatedAt: item.CreatedAt, UpdatedAt: item.UpdatedAt}
}

func configDTO(config *clashclient.Config) map[string]any {
	return map[string]any{"port": config.Port, "socks-port": config.SocksPort, "redir-port": config.RedirPort, "tproxy-port": config.TProxyPort, "mixed-port": config.MixedPort, "allow-lan": config.AllowLan, "bind-address": config.BindAddress, "mode": config.Mode, "log-level": config.LogLevel, "ipv6": config.Ipv6, "external-controller": config.ExternalController}
}

type clashStream struct {
	cancel                       context.CancelFunc
	traffic                      <-chan clashclient.Traffic
	connections                  <-chan clashclient.ConnectionsResponse
	trafficOpen, connectionsOpen bool
}

func (s *clashStream) Close() { s.cancel() }
func (s *clashStream) Next(ctx context.Context) (string, any, error) {
	for {
		if !s.trafficOpen && !s.connectionsOpen {
			return "", nil, fmt.Errorf("Clash streams closed")
		}
		select {
		case <-ctx.Done():
			return "", nil, ctx.Err()
		case <-time.After(15 * time.Second):
			return "ping", map[string]bool{"alive": true}, nil
		case value, ok := <-s.traffic:
			if !ok {
				s.trafficOpen, s.traffic = false, nil
				continue
			}
			return "traffic", map[string]any{"upload_speed": value.Up, "download_speed": value.Down}, nil
		case value, ok := <-s.connections:
			if !ok {
				s.connectionsOpen, s.connections = false, nil
				continue
			}
			return "connections", map[string]any{"connections": value.Connections, "upload_total": value.UploadTotal, "download_total": value.DownloadTotal, "conn_count": len(value.Connections)}, nil
		}
	}
}

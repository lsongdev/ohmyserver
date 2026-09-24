package web

import (
	"context"
	"errors"
	"fmt"
	"strings"

	webservermodule "github.com/lsongdev/ohmyserver/modules/webserver"
	"github.com/lsongdev/ohmyserver/modules/webserver/web/repository"
)

// ApplyPolicy is an optional host check run before a stored configuration is
// applied. Hosts use it for shared port-conflict and service-preference rules.
type ApplyPolicy func(context.Context) error

// LocalService implements the module's API against its own repository and
// runtime. It can run in the shell or in a standalone command.
type LocalService struct {
	repository   *repository.Repository
	runtime      *Runtime
	certificates webservermodule.CertificateSource
	applyPolicy  ApplyPolicy
}

func NewLocalService(repo *repository.Repository, runtime *Runtime, certificates webservermodule.CertificateSource, applyPolicy ApplyPolicy) *LocalService {
	return &LocalService{repository: repo, runtime: runtime, certificates: certificates, applyPolicy: applyPolicy}
}

func (s *LocalService) WebListeners(ctx context.Context) ([]Listener, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	items, err := s.repository.ListListeners(ctx)
	if err != nil {
		return nil, err
	}
	if s.certificates == nil {
		return items, nil
	}
	summaries, err := s.certificates.ListSummaries(ctx)
	if err != nil {
		return nil, err
	}
	for i := range items {
		if items[i].CertificateID == nil {
			continue
		}
		for _, item := range summaries {
			if item.ID == *items[i].CertificateID {
				items[i].CertificateName = item.Domains
				break
			}
		}
	}
	return items, nil
}

func (s *LocalService) WebHosts(ctx context.Context) ([]Host, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	items, err := s.repository.ListHosts(ctx)
	if err != nil {
		return nil, err
	}
	var summaries []webservermodule.CertificateSummary
	if s.certificates != nil {
		summaries, err = s.certificates.ListSummaries(ctx)
		if err != nil {
			return nil, err
		}
	}
	for i := range items {
		items[i].Routes, err = s.repository.ListRoutesByHost(ctx, items[i].ID)
		if err != nil {
			return nil, err
		}
		if items[i].CertificateID != nil && s.certificates != nil {
			for _, item := range summaries {
				if item.ID == *items[i].CertificateID {
					items[i].CertificateName = item.Domains
					break
				}
			}
		}
	}
	return items, nil
}

func (s *LocalService) WebPolicies(ctx context.Context) ([]Policy, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	return s.repository.ListPolicies(ctx)
}

func (s *LocalService) WebCertificates(ctx context.Context) ([]Certificate, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if s.certificates == nil {
		return []Certificate{}, nil
	}
	summaries, err := s.certificates.ListSummaries(ctx)
	if err != nil {
		return nil, err
	}
	items := make([]Certificate, 0, len(summaries))
	for _, item := range summaries {
		if item.Status == "active" {
			items = append(items, Certificate{ID: item.ID, Domains: item.Domains})
		}
	}
	return items, nil
}

func (s *LocalService) CreateWebListenerAPI(ctx context.Context, item Listener) (Listener, error) {
	if err := ctx.Err(); err != nil {
		return Listener{}, err
	}
	if item.Name == "" || item.Address == "" || (item.Protocol != "http" && item.Protocol != "https") {
		return Listener{}, errors.New("name, address and a supported protocol are required")
	}
	if item.Protocol == "https" && item.CertificateID == nil {
		return Listener{}, errors.New("HTTPS listeners require a default certificate")
	}
	if item.MinTLSVersion == "" {
		item.MinTLSVersion = "TLS1.2"
	}
	id, err := s.repository.CreateListener(ctx, item)
	if err != nil {
		return Listener{}, err
	}
	item.ID = id
	if err := s.apply(ctx); err != nil {
		_ = s.repository.DeleteListener(context.WithoutCancel(ctx), id)
		return Listener{}, err
	}
	return item, nil
}

func (s *LocalService) UpdateWebListenerAPI(ctx context.Context, id int64, item Listener) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if item.Name == "" || item.Address == "" || (item.Protocol != "http" && item.Protocol != "https") {
		return errors.New("name, address and a supported protocol are required")
	}
	if item.Protocol == "https" && item.CertificateID == nil {
		return errors.New("HTTPS listeners require a default certificate")
	}
	old, err := s.repository.GetListener(ctx, id)
	if err != nil {
		return err
	}
	item.ID = id
	if item.MinTLSVersion == "" {
		item.MinTLSVersion = "TLS1.2"
	}
	if err := s.repository.UpdateListener(ctx, item); err != nil {
		return err
	}
	if err := s.apply(ctx); err != nil {
		_ = s.repository.UpdateListener(context.WithoutCancel(ctx), *old)
		return err
	}
	return nil
}

func (s *LocalService) DeleteWebListenerAPI(ctx context.Context, id int64) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if _, err := s.repository.GetListener(ctx, id); err != nil {
		return err
	}
	if err := s.repository.DeleteListener(ctx, id); err != nil {
		return err
	}
	return s.apply(ctx)
}

func (s *LocalService) CreateWebHostAPI(ctx context.Context, item Host) (Host, error) {
	if err := ctx.Err(); err != nil {
		return Host{}, err
	}
	if strings.TrimSpace(item.Name) == "" || strings.TrimSpace(item.Hostnames) == "" {
		return Host{}, errors.New("name and hostnames are required")
	}
	id, err := s.repository.CreateHost(ctx, item)
	if err != nil {
		return Host{}, err
	}
	item.ID = id
	if err := s.apply(ctx); err != nil {
		_ = s.repository.DeleteHost(context.WithoutCancel(ctx), id)
		return Host{}, err
	}
	return item, nil
}

func (s *LocalService) UpdateWebHostAPI(ctx context.Context, id int64, item Host) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if strings.TrimSpace(item.Name) == "" || strings.TrimSpace(item.Hostnames) == "" {
		return errors.New("name and hostnames are required")
	}
	old, err := s.repository.GetHost(ctx, id)
	if err != nil {
		return err
	}
	item.ID = id
	if err := s.repository.UpdateHost(ctx, item); err != nil {
		return err
	}
	if err := s.apply(ctx); err != nil {
		_ = s.repository.UpdateHost(context.WithoutCancel(ctx), *old)
		return err
	}
	return nil
}

func (s *LocalService) DeleteWebHostAPI(ctx context.Context, id int64) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if _, err := s.repository.GetHost(ctx, id); err != nil {
		return err
	}
	if err := s.repository.DeleteHost(ctx, id); err != nil {
		return err
	}
	return s.apply(ctx)
}

func (s *LocalService) CreateWebRouteAPI(ctx context.Context, hostID int64, item Route) (Route, error) {
	if err := ctx.Err(); err != nil {
		return Route{}, err
	}
	if _, err := s.repository.GetHost(ctx, hostID); err != nil {
		return Route{}, err
	}
	item.HostID = hostID
	if err := validateRoute(&item); err != nil {
		return Route{}, err
	}
	id, err := s.repository.CreateRoute(ctx, item)
	if err != nil {
		return Route{}, err
	}
	item.ID = id
	if err := s.apply(ctx); err != nil {
		_ = s.repository.DeleteRoute(context.WithoutCancel(ctx), id)
		return Route{}, err
	}
	return item, nil
}

func (s *LocalService) UpdateWebRouteAPI(ctx context.Context, id int64, item Route) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	old, err := s.repository.GetRoute(ctx, id)
	if err != nil {
		return err
	}
	item.ID = id
	if item.HostID == 0 {
		item.HostID = old.HostID
	}
	if err := validateRoute(&item); err != nil {
		return err
	}
	if err := s.repository.UpdateRoute(ctx, item); err != nil {
		return err
	}
	if err := s.apply(ctx); err != nil {
		_ = s.repository.UpdateRoute(context.WithoutCancel(ctx), *old)
		return err
	}
	return nil
}

func (s *LocalService) DeleteWebRouteAPI(ctx context.Context, id int64) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if _, err := s.repository.GetRoute(ctx, id); err != nil {
		return err
	}
	if err := s.repository.DeleteRoute(ctx, id); err != nil {
		return err
	}
	return s.apply(ctx)
}

func (s *LocalService) CreateWebPolicyAPI(ctx context.Context, item Policy) (Policy, error) {
	if err := ctx.Err(); err != nil {
		return Policy{}, err
	}
	if err := validatePolicy(item); err != nil {
		return Policy{}, err
	}
	id, err := s.repository.CreatePolicy(ctx, item)
	if err != nil {
		return Policy{}, err
	}
	item.ID = id
	if err := s.apply(ctx); err != nil {
		_ = s.repository.DeletePolicy(context.WithoutCancel(ctx), id)
		return Policy{}, err
	}
	return item, nil
}

func (s *LocalService) UpdateWebPolicyAPI(ctx context.Context, id int64, item Policy) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if err := validatePolicy(item); err != nil {
		return err
	}
	old, err := s.repository.GetPolicy(ctx, id)
	if err != nil {
		return err
	}
	item.ID = id
	if err := s.repository.UpdatePolicy(ctx, item); err != nil {
		return err
	}
	if err := s.apply(ctx); err != nil {
		_ = s.repository.UpdatePolicy(context.WithoutCancel(ctx), *old)
		return err
	}
	return nil
}

func (s *LocalService) DeleteWebPolicyAPI(ctx context.Context, id int64) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if _, err := s.repository.GetPolicy(ctx, id); err != nil {
		return err
	}
	if err := s.repository.DeletePolicy(ctx, id); err != nil {
		return err
	}
	return s.apply(ctx)
}

func (s *LocalService) apply(ctx context.Context) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if s.applyPolicy != nil {
		if err := s.applyPolicy(ctx); err != nil {
			return err
		}
	}
	if s.runtime != nil {
		return s.runtime.Apply(ctx)
	}
	return nil
}

func validatePolicy(item Policy) error {
	if strings.TrimSpace(item.Name) == "" {
		return errors.New("policy name is required")
	}
	if item.MaxBodyBytes < 0 || item.RequestTimeoutSeconds < 0 || item.RateRequestsPerSecond < 0 || item.RateBurst < 0 || item.RateMaxKeys < 0 || item.RateIdleTimeoutSeconds < 0 || item.ConcurrencyMax < 0 {
		return errors.New("policy limits must be non-negative")
	}
	if item.RateKey != "" && item.RateKey != "client_ip" && item.RateKey != "global" {
		return errors.New("rate key must be client_ip or global")
	}
	if item.RateRequestsPerSecond > 0 && item.RateBurst <= 0 {
		return errors.New("rate burst is required when rate limiting is enabled")
	}
	return nil
}

func validateRoute(item *Route) error {
	if strings.TrimSpace(item.Name) == "" {
		return errors.New("route name is required")
	}
	if item.Path == "" {
		item.Path = "/"
	}
	item.Method = strings.ToUpper(item.Method)
	switch item.HandlerType {
	case "reverse_proxy":
		if strings.TrimSpace(item.UpstreamURL) == "" {
			return errors.New("upstream URL is required")
		}
	case "file_server":
		if strings.TrimSpace(item.Root) == "" {
			return errors.New("root is required")
		}
	case "static_response":
		if item.ResponseStatus < 100 || item.ResponseStatus > 599 {
			return errors.New("valid response status is required")
		}
	case "redirect":
		if strings.TrimSpace(item.RedirectLocation) == "" {
			return errors.New("redirect location is required")
		}
	case "worker":
		if strings.TrimSpace(item.WorkerScript) == "" {
			return errors.New("worker script is required")
		}
	default:
		return fmt.Errorf("invalid handler type %q", item.HandlerType)
	}
	if item.RequestTimeoutSeconds < 0 || item.ResponseHeaderTimeoutSeconds < 0 || item.MaxConnsPerHost < 0 {
		return errors.New("route limits must be non-negative")
	}
	return nil
}

var _ Service = (*LocalService)(nil)

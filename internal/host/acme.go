package host

import (
	"context"

	acmemodule "github.com/lsongdev/ohmyserver/modules/acme"
	acmeweb "github.com/lsongdev/ohmyserver/modules/acme/web"
)

func (s *Server) ACMERepository() *acmeweb.Repository {
	if s.acmeRepository != nil {
		return s.acmeRepository
	}
	return acmeweb.NewRepository(s.db)
}

// ACMEEngine exposes the feature service to the composition root. The engine
// owns ACME workers and persistence; the host supplies DNS challenges.
func (s *Server) ACMEEngine() *acmemodule.Engine { return s.acmeEngine }

func (s *Server) onACMECertificateUpdated(ctx context.Context, _ int64) error {
	if !s.serviceDesiredEnabled("web", true) {
		return nil
	}
	return s.applyYabber(ctx)
}

type webCertificateSource interface {
	GetSummary(context.Context, int64) (acmeweb.CertificateSummary, error)
	ListSummaries(context.Context) ([]acmeweb.CertificateSummary, error)
	LoadTLSMaterial(context.Context, int64) (acmeweb.TLSMaterial, error)
}

func (s *Server) webCertificateSource() webCertificateSource {
	return s.ACMERepository()
}

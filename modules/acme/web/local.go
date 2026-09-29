package web

import (
	"context"
	"net/http"

	acme "github.com/lsongdev/ohmyserver/modules/acme"
)

// LocalModule assembles ACME persistence, issuance workers, and HTTP endpoints.
// DNS challenges and certificate reload behavior remain caller supplied.
type LocalModule struct {
	repository *Repository
	engine     *acme.Engine
	http       *Module
}

// NewLocalModule composes a repository that has already been initialized by
// the host's schema lifecycle.
func NewLocalModule(ctx context.Context, repository *Repository, dns acme.DNSChallenge, updated acme.CertificateUpdated) *LocalModule {
	engine := acme.NewEngine(ctx, repository, dns, updated)
	return &LocalModule{repository: repository, engine: engine, http: New(engine)}
}

func (m *LocalModule) Repository() *Repository { return m.repository }
func (m *LocalModule) Engine() *acme.Engine    { return m.engine }
func (m *LocalModule) Web() *Module            { return m.http }
func (m *LocalModule) Handler() http.Handler   { return m.http.Handler() }

func (m *LocalModule) Close() error { return m.engine.Close() }

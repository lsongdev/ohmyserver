package web

import (
	"context"
	"database/sql"
)

// LocalModule composes the DHCP repository, runtime service, and HTTP handler.
// A host may provide an API service adapter to coordinate application-wide
// policy while the module retains ownership of its local runtime.
type LocalModule struct {
	*Module
	repository *Repository
	service    *LocalService
}

func NewLocalModule(ctx context.Context, db *sql.DB, policy ApplyPolicy, apiServices ...Service) *LocalModule {
	repository := NewRepository(db)
	service := NewLocalService(ctx, repository, policy)
	var apiService Service = service
	if len(apiServices) > 0 && apiServices[0] != nil {
		apiService = apiServices[0]
	}
	return &LocalModule{Module: New(repository, apiService), repository: repository, service: service}
}

func (m *LocalModule) Initialize(ctx context.Context) error { return m.repository.Initialize(ctx) }
func (m *LocalModule) Start(ctx context.Context) error      { return m.service.Start(ctx) }
func (m *LocalModule) Repository() *Repository              { return m.repository }
func (m *LocalModule) Service() *LocalService               { return m.service }
func (m *LocalModule) Close() error                         { return m.service.Close() }

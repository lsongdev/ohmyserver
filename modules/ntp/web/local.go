package web

import (
	"context"
	"database/sql"
)

// LocalModule composes the SQLite repository, NTP runtime service, and HTTP
// module used by the standalone command and the application host.
type LocalModule struct {
	*Module
	repository *Repository
	service    *LocalService
}

func NewLocalModule(db *sql.DB, policy ApplyPolicy) *LocalModule {
	repository := NewRepository(db)
	service := NewLocalService(repository, policy)
	return &LocalModule{Module: New(repository, service), repository: repository, service: service}
}

func (m *LocalModule) Initialize(ctx context.Context) error { return m.repository.Initialize(ctx) }
func (m *LocalModule) Repository() *Repository              { return m.repository }
func (m *LocalModule) Service() *LocalService               { return m.service }
func (m *LocalModule) Close() error                         { return m.service.Close() }

package web

import (
	"context"
	"database/sql"
)

// LocalModule composes the IPMI profile repository, service, and HTTP handler.
type LocalModule struct {
	*Module
	repository *Repository
	service    *LocalService
}

func NewLocalModule(db *sql.DB) *LocalModule {
	repository := NewRepository(db)
	service := NewLocalService(repository)
	return &LocalModule{Module: New(service), repository: repository, service: service}
}

func (m *LocalModule) Initialize(ctx context.Context) error { return m.repository.Initialize(ctx) }
func (m *LocalModule) Repository() *Repository              { return m.repository }
func (m *LocalModule) Service() *LocalService               { return m.service }

package web

import (
	"context"
	"database/sql"
)

// LocalModule composes the ESXi repository, service, and HTTP handler.
type LocalModule struct {
	*Module
	repository *Repository
	service    *Service
}

func NewLocalModule(db *sql.DB) *LocalModule {
	repository := NewRepository(db)
	service := NewService(repository)
	return &LocalModule{Module: New(service), repository: repository, service: service}
}

func (m *LocalModule) Initialize(ctx context.Context) error { return m.repository.Initialize(ctx) }
func (m *LocalModule) Repository() *Repository              { return m.repository }
func (m *LocalModule) Service() *Service                    { return m.service }

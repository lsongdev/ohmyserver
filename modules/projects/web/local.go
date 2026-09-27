package web

import (
	"context"
	"database/sql"
	"net/http"
)

// LocalModule composes the module's repository, deployment manager, service,
// and HTTP handler for a local standalone installation.
type LocalModule struct {
	module  *Module
	manager *DeploymentManager
}

func NewLocalModule(ctx context.Context, db *sql.DB, runtime DeploymentRuntime, workspace string) (*LocalModule, error) {
	repository := NewRepository(db)
	if err := repository.Initialize(ctx); err != nil {
		return nil, err
	}
	if err := repository.RecoverInterruptedDeployments(ctx); err != nil {
		return nil, err
	}
	manager := NewDeploymentManager(ctx, repository, runtime, workspace)
	return &LocalModule{module: New(repository, manager), manager: manager}, nil
}

func (m *LocalModule) Handler() http.Handler           { return m.module.Handler() }
func (m *LocalModule) StandaloneHandler() http.Handler { return m.module.StandaloneHandler() }
func (m *LocalModule) Close()                          { m.manager.Close() }

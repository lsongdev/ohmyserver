package web

import (
	"net/http"
)

// LocalModule composes OpenWrt's repository, local vendor client service, and
// HTTP module. A host can provide an initialized repository from its shared DB.
type LocalModule struct {
	repository *Repository
	module     *Module
}

func NewLocalModule(repository *Repository) *LocalModule {
	return &LocalModule{repository: repository, module: New(repository, NewLocalService(repository))}
}

func (m *LocalModule) Repository() *Repository { return m.repository }
func (m *LocalModule) Web() *Module            { return m.module }
func (m *LocalModule) Handler() http.Handler   { return m.module.Handler() }

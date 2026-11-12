package api

import (
	"context"
	"net/http"

	"github.com/lsongdev/ohmyserver/modules/netmgr/system"
)

// bridges is the backend as a bridge maker, or a refusal that says why not.
func (s *Service) bridges() (system.BridgeBackend, error) {
	if b, ok := s.backend.(system.BridgeBackend); ok {
		return b, nil
	}
	return nil, &errInUse{"Bridges are made through NetworkManager, which is not what runs on this machine."}
}

func (s *Service) registerBridges(handle func(string, func(*http.Request) (any, error))) {
	handle("GET /bridges", func(r *http.Request) (any, error) {
		b, err := s.bridges()
		if err != nil {
			return nil, err
		}
		return b.Bridges(r.Context())
	})
	handle("GET /bridges/{name}", func(r *http.Request) (any, error) {
		b, err := s.bridges()
		if err != nil {
			return nil, err
		}
		bridge, err := b.Bridge(r.Context(), r.PathValue("name"))
		if err == nil && bridge == nil {
			err = system.ErrNotFound
		}
		return bridge, err
	})
	handle("PUT /bridges/{name}", s.saveBridge)
	handle("DELETE /bridges/{name}", s.deleteBridge)
}

type bridgeBody struct {
	Version string              `json:"version"` // empty when making a new bridge
	Config  system.BridgeConfig `json:"config"`
}

func (s *Service) saveBridge(r *http.Request) (any, error) {
	bb, err := s.bridges()
	if err != nil {
		return nil, err
	}
	name := r.PathValue("name")
	var body bridgeBody
	if err := decode(r, &body); err != nil {
		return nil, err
	}
	cfg, err := body.Config.Normalize()
	if err != nil {
		return nil, err
	}
	links, err := system.Links(r.Context(), s.run)
	if err != nil {
		return nil, err
	}
	current, err := bb.Bridge(r.Context(), name)
	if err != nil {
		return nil, err
	}
	if fields := system.BridgeProblems(name, current, cfg, links); len(fields) > 0 {
		return nil, &system.ValidationError{Fields: fields}
	}
	// Commands continue if the change ends this client's connection.
	ctx, cancel := context.WithTimeout(context.Background(), applyTimeout)
	defer cancel()
	return s.trial.start(ctx, change{
		subject: name,
		begin: func(ctx context.Context) (string, system.Backend, system.Undo, error) {
			now, err := bb.Bridge(ctx, name)
			if err != nil {
				return "", nil, system.Undo{}, err
			}
			// A new bridge has no version, so one that appeared meanwhile
			// is a conflict, as is one that changed.
			if (now == nil) != (body.Version == "") || (now != nil && now.Version != body.Version) {
				return "", nil, system.Undo{}, system.ErrConflict
			}
			undo, err := bb.PrepareBridge(ctx, name, cfg)
			return "Bridge " + name, s.backend, undo, err
		},
		apply: func(ctx context.Context, _ system.Backend) error { return bb.ApplyBridge(ctx, name, cfg) },
	}, s.window)
}

func (s *Service) deleteBridge(r *http.Request) (any, error) {
	bb, err := s.bridges()
	if err != nil {
		return nil, err
	}
	name, version := r.PathValue("name"), r.URL.Query().Get("version")
	ctx, cancel := context.WithTimeout(context.Background(), applyTimeout)
	defer cancel()
	return s.trial.start(ctx, change{
		subject: name,
		begin: func(ctx context.Context) (string, system.Backend, system.Undo, error) {
			now, err := bb.Bridge(ctx, name)
			if err != nil {
				return "", nil, system.Undo{}, err
			}
			if now == nil {
				return "", nil, system.Undo{}, system.ErrNotFound
			}
			if now.Version != version {
				return "", nil, system.Undo{}, system.ErrConflict
			}
			undo, err := bb.PrepareDeleteBridge(ctx, name)
			return "Removing bridge " + name, s.backend, undo, err
		},
		apply: func(ctx context.Context, _ system.Backend) error { return bb.DeleteBridge(ctx, name) },
	}, s.window)
}

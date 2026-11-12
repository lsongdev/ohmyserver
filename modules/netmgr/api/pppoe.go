package api

import (
	"context"
	"net/http"
	"strings"

	"github.com/lsongdev/ohmyserver/modules/netmgr/system"
)

// pppoe is the backend as a dialer, or a refusal that says why not.
func (s *Service) pppoe() (system.PPPoEBackend, error) {
	if b, ok := s.backend.(system.PPPoEBackend); ok {
		return b, nil
	}
	return nil, &errInUse{"PPPoE is managed through NetworkManager, which is not what runs on this machine."}
}

// PPPoE is a connection as the page shows it.
type PPPoE struct {
	system.PPPoE
	// Protected: hanging it up could cut this machine or this client off, so
	// it is not hung up or forgotten from here.
	Protected bool `json:"protected"`
}

// PPPoEOverview answers GET /pppoe.
type PPPoEOverview struct {
	Ready       bool     `json:"ready"`
	Missing     []string `json:"missing,omitempty"`
	Parents     []string `json:"parents"` // Ethernet interfaces a connection can dial through
	Connections []PPPoE  `json:"connections"`
}

func (s *Service) registerPPPoE(handle func(string, func(*http.Request) (any, error))) {
	handle("GET /pppoe", s.pppoeOverview)
	handle("PUT /pppoe/{name}", s.pppoeSave)
	handle("DELETE /pppoe/{name}", s.pppoeForget)
	handle("POST /pppoe/{name}/dial", s.pppoeDial)
	handle("POST /pppoe/{name}/hangup", s.pppoeHangup)
}

func (s *Service) pppoeReady(ctx context.Context) system.Readiness {
	if fb, ok := s.backend.(featureBackend); ok {
		return fb.DetectFeatures(ctx).PPPoE
	}
	return system.Readiness{}
}

func (s *Service) pppoeOverview(r *http.Request) (any, error) {
	pb, err := s.pppoe()
	if err != nil {
		return nil, err
	}
	list, err := pb.PPPoEList(r.Context())
	if err != nil {
		return nil, err
	}
	links, _ := system.Links(r.Context(), s.run)
	view := PPPoEOverview{Parents: []string{}, Connections: []PPPoE{}}
	ready := s.pppoeReady(r.Context())
	view.Ready, view.Missing = ready.Ready, ready.Missing
	protected := map[string]bool{}
	client := s.clientDevice(r)
	for _, l := range links {
		protected[l.Name] = l.Uplink || l.Name == client
		if l.Kind == "ethernet" && system.EditableReason(l) == "" {
			view.Parents = append(view.Parents, l.Name)
		}
	}
	for _, c := range list {
		view.Connections = append(view.Connections, PPPoE{PPPoE: c, Protected: c.Active && protected[c.Device]})
	}
	return view, nil
}

// connection finds one by the name in the path.
func (s *Service) connection(r *http.Request, pb system.PPPoEBackend) (*PPPoE, error) {
	overview, err := s.pppoeOverview(r)
	if err != nil {
		return nil, err
	}
	for _, c := range overview.(PPPoEOverview).Connections {
		if c.Name == r.PathValue("name") {
			return &c, nil
		}
	}
	return nil, system.ErrNotFound
}

func (s *Service) needReady(ctx context.Context) error {
	if ready := s.pppoeReady(ctx); !ready.Ready {
		return &errInUse{"Dialing needs " + strings.Join(ready.Missing, " and ") + ", which is not installed."}
	}
	return nil
}

// pppoeSave saves and dials on trial: a login that fails, or is not kept,
// leaves things as they were. The password stays in this closure; the trial
// record holds only what to delete or restore.
func (s *Service) pppoeSave(r *http.Request) (any, error) {
	pb, err := s.pppoe()
	if err != nil {
		return nil, err
	}
	var req system.PPPoERequest
	if err := decode(r, &req); err != nil {
		return nil, err
	}
	req.Name = r.PathValue("name")
	if req, err = req.Normalize(); err != nil {
		return nil, err
	}
	if err := s.needReady(r.Context()); err != nil {
		return nil, err
	}
	parent, err := system.FindLink(r.Context(), s.run, req.Parent)
	if err != nil || parent.Kind != "ethernet" || system.EditableReason(*parent) != "" {
		return nil, &system.ValidationError{Fields: map[string]string{"parent": req.Parent + " is not an Ethernet interface that can dial"}}
	}
	ctx, cancel := context.WithTimeout(context.Background(), connectTimeout)
	defer cancel()
	return s.trial.start(ctx, change{
		subject: req.Name,
		begin: func(ctx context.Context) (string, system.Backend, system.Undo, error) {
			undo, err := pb.PreparePPPoE(ctx, req)
			return "PPPoE " + req.Name, s.backend, undo, err
		},
		apply: func(ctx context.Context, _ system.Backend) error { return pb.SavePPPoE(ctx, req) },
	}, s.window)
}

func (s *Service) pppoeDial(r *http.Request) (any, error) {
	pb, err := s.pppoe()
	if err != nil {
		return nil, err
	}
	if err := s.needReady(r.Context()); err != nil {
		return nil, err
	}
	c, err := s.connection(r, pb)
	if err != nil {
		return nil, err
	}
	if c.Active {
		return nil, &errInUse{c.Name + " is up already"}
	}
	ctx, cancel := context.WithTimeout(context.Background(), connectTimeout)
	defer cancel()
	return s.trial.start(ctx, change{
		subject: c.Name,
		begin: func(ctx context.Context) (string, system.Backend, system.Undo, error) {
			undo, err := pb.PrepareDial(ctx, c.Name)
			return "PPPoE " + c.Name, s.backend, undo, err
		},
		apply: func(ctx context.Context, _ system.Backend) error { return pb.Dial(ctx, c.Name) },
	}, s.window)
}

func (s *Service) pppoeHangup(r *http.Request) (any, error) {
	pb, err := s.pppoe()
	if err != nil {
		return nil, err
	}
	c, err := s.connection(r, pb)
	if err != nil {
		return nil, err
	}
	if err := s.refuseWhileInUse(c); err != nil {
		return nil, err
	}
	ctx, cancel := context.WithTimeout(context.Background(), applyTimeout)
	defer cancel()
	if err := pb.Hangup(ctx, c.Name); err != nil {
		return nil, err
	}
	s.logger.Info("pppoe hung up", "name", c.Name)
	return map[string]bool{"active": false}, nil
}

// pppoeForget deletes a connection and its password. That cannot be undone,
// so it is not on trial: it is refused for a connection made some other way,
// while a change is on trial, and for one that is up and protected.
func (s *Service) pppoeForget(r *http.Request) (any, error) {
	pb, err := s.pppoe()
	if err != nil {
		return nil, err
	}
	c, err := s.connection(r, pb)
	if err != nil {
		return nil, err
	}
	if !c.Managed {
		return nil, &errInUse{c.Name + " was made some other way, so it is not deleted from here."}
	}
	if err := s.refuseWhileInUse(c); err != nil {
		return nil, err
	}
	ctx, cancel := context.WithTimeout(context.Background(), applyTimeout)
	defer cancel()
	if err := pb.ForgetPPPoE(ctx, c.Name); err != nil {
		return nil, err
	}
	s.logger.Info("pppoe forgotten", "name", c.Name)
	return map[string]string{"forgotten": c.Name}, nil
}

func (s *Service) refuseWhileInUse(c *PPPoE) error {
	switch {
	case s.trial.status().Trial != nil:
		return &errInUse{"A change is on trial; keep or revert it first."}
	case c.Protected:
		return &errInUse{c.Name + " carries this machine's default route or your connection; hanging it up could leave the panel unreachable."}
	}
	return nil
}

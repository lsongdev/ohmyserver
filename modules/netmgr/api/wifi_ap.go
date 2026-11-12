package api

import (
	"context"
	"net/http"

	"github.com/lsongdev/ohmyserver/modules/netmgr/system"
)

// A radio's hotspot is found by type assertion, so a method missing from the
// real backend would make hotspots look unsupported instead of failing to build.
var _ system.WifiAPBackend = (*system.NetworkManager)(nil)

func (s *Service) wifiAP() (system.WifiAPBackend, error) {
	if b, ok := s.backend.(system.WifiAPBackend); ok {
		return b, nil
	}
	return nil, &errInUse{"Hotspots are managed through NetworkManager, which is not what runs on this machine."}
}

// HotspotView answers GET /wifi/{name}/hotspot.
type HotspotView struct {
	Hotspot *system.WifiAP `json:"hotspot"` // nil when none is set up
	Capable bool           `json:"capable"` // the radio can serve a network
	Bridges []string       `json:"bridges"` // a hotspot can be a port of one of these
	// Protected: the radio carries this machine's default route or this
	// client's connection, so starting or stopping a hotspot on it ends that.
	Protected bool `json:"protected"`
}

func (s *Service) registerWifiAP(handle func(string, func(*http.Request) (any, error))) {
	handle("GET /wifi/{name}/hotspot", s.hotspotGet)
	handle("PUT /wifi/{name}/hotspot", s.hotspotSave)
	handle("POST /wifi/{name}/hotspot/stop", s.hotspotStop)
}

// radio finds the device and says whether it can serve a network.
func (s *Service) radio(ctx context.Context, name string) (*system.WifiDevice, error) {
	wb, err := s.wifi()
	if err != nil {
		return nil, err
	}
	devices, err := wb.WifiDevices(ctx)
	if err != nil {
		return nil, err
	}
	for i := range devices {
		if devices[i].Name == name {
			return &devices[i], nil
		}
	}
	return nil, system.ErrNotFound
}

func (s *Service) protectedRadio(r *http.Request, name string) bool {
	if name == s.clientDevice(r) {
		return true
	}
	l, err := system.FindLink(r.Context(), s.run, name)
	return err == nil && l.Uplink
}

func (s *Service) hotspotGet(r *http.Request) (any, error) {
	ab, err := s.wifiAP()
	if err != nil {
		return nil, err
	}
	name := r.PathValue("name")
	dev, err := s.radio(r.Context(), name)
	if err != nil {
		return nil, err
	}
	ap, err := ab.WifiAP(r.Context(), name)
	if err != nil {
		return nil, err
	}
	view := HotspotView{Hotspot: ap, Capable: dev.AP, Bridges: []string{}, Protected: s.protectedRadio(r, name)}
	if bb, err := s.bridges(); err == nil {
		list, _ := bb.Bridges(r.Context())
		for _, b := range list {
			view.Bridges = append(view.Bridges, b.Name)
		}
	}
	return view, nil
}

// hotspotSave starts a hotspot on trial. The radio stops being a client, so
// the network it had joined is put back if the change is not kept. The
// password stays in this closure; the trial record holds only what to delete
// or restore.
func (s *Service) hotspotSave(r *http.Request) (any, error) {
	ab, err := s.wifiAP()
	if err != nil {
		return nil, err
	}
	name := r.PathValue("name")
	var req system.WifiAPRequest
	if err := decode(r, &req); err != nil {
		return nil, err
	}
	if req, err = req.Normalize(); err != nil {
		return nil, err
	}
	dev, err := s.radio(r.Context(), name)
	if err != nil {
		return nil, err
	}
	if !dev.AP {
		return nil, &errInUse{name + " cannot serve a network: its driver has no access point mode."}
	}
	ctx, cancel := context.WithTimeout(context.Background(), connectTimeout)
	defer cancel()
	return s.trial.start(ctx, change{
		subject: name,
		begin: func(ctx context.Context) (string, system.Backend, system.Undo, error) {
			undo, err := ab.PrepareWifiAP(ctx, name, req)
			return "Hotspot " + req.SSID, s.backend, undo, err
		},
		apply: func(ctx context.Context, _ system.Backend) error { return ab.SaveWifiAP(ctx, name, req) },
	}, s.window)
}

// hotspotStop ends a hotspot. It is refused while a change is on trial, and
// when this client is connected through the radio, which stopping would end.
func (s *Service) hotspotStop(r *http.Request) (any, error) {
	ab, err := s.wifiAP()
	if err != nil {
		return nil, err
	}
	name := r.PathValue("name")
	if _, err := s.radio(r.Context(), name); err != nil {
		return nil, err
	}
	ap, err := ab.WifiAP(r.Context(), name)
	if err != nil {
		return nil, err
	}
	if ap == nil {
		return nil, system.ErrNotFound
	}
	switch {
	case s.trial.status().Trial != nil:
		return nil, &errInUse{"A change is on trial; keep or revert it first."}
	case ap.Active && name == s.clientDevice(r):
		return nil, &errInUse{"You are connected to this panel through " + name + "; stopping its hotspot would end that connection."}
	}
	ctx, cancel := context.WithTimeout(context.Background(), applyTimeout)
	defer cancel()
	if err := ab.StopWifiAP(ctx, name); err != nil {
		return nil, err
	}
	s.logger.Info("hotspot stopped", "device", name)
	return map[string]bool{"active": false}, nil
}

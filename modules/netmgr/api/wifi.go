package api

import (
	"context"
	"net/http"
	"regexp"

	"github.com/lsongdev/ohmyserver/modules/netmgr/system"
)

// wifi is the backend as a Wi-Fi client, or a refusal that says why not.
func (s *Service) wifi() (system.WifiBackend, error) {
	if b, ok := s.backend.(system.WifiBackend); ok {
		return b, nil
	}
	return nil, &errInUse{"Wi-Fi is managed through NetworkManager, which is not what runs on this machine."}
}

// WifiDevice is a radio as the page shows it.
type WifiDevice struct {
	system.WifiDevice
	// Protected: leaving its network could cut this machine or this client
	// off, so it is not disconnected or forgotten from here.
	Protected bool `json:"protected"`
}

// WifiOverview answers GET /wifi.
type WifiOverview struct {
	Devices []WifiDevice       `json:"devices"`
	Saved   []system.WifiSaved `json:"saved"`
}

var uuidPattern = regexp.MustCompile(`^[0-9a-fA-F-]{8,40}$`)

func (s *Service) registerWifi(handle func(string, func(*http.Request) (any, error))) {
	handle("GET /wifi", s.wifiOverview)
	handle("GET /wifi/{name}/networks", s.wifiNetworks)
	handle("PUT /wifi/{name}/connection", s.wifiConnect)
	handle("DELETE /wifi/saved/{uuid}", s.wifiForget)
}

func (s *Service) wifiOverview(r *http.Request) (any, error) {
	wb, err := s.wifi()
	if err != nil {
		return nil, err
	}
	devices, err := wb.WifiDevices(r.Context())
	if err != nil {
		return nil, err
	}
	saved, err := wb.WifiSaved(r.Context())
	if err != nil {
		return nil, err
	}
	links, _ := system.Links(r.Context(), s.run)
	protected := map[string]bool{}
	client := s.clientDevice(r)
	for _, l := range links {
		protected[l.Name] = l.Uplink || l.Name == client
	}
	view := WifiOverview{Devices: []WifiDevice{}, Saved: saved}
	for _, d := range devices {
		view.Devices = append(view.Devices, WifiDevice{WifiDevice: d, Protected: protected[d.Name]})
	}
	return view, nil
}

// device finds a radio by name, so a name never reaches a command unless
// NetworkManager lists it.
func (s *Service) device(ctx context.Context, wb system.WifiBackend, name string) error {
	devices, err := wb.WifiDevices(ctx)
	if err != nil {
		return err
	}
	for _, d := range devices {
		if d.Name == name {
			return nil
		}
	}
	return system.ErrNotFound
}

func (s *Service) wifiNetworks(r *http.Request) (any, error) {
	wb, err := s.wifi()
	if err != nil {
		return nil, err
	}
	name := r.PathValue("name")
	if err := s.device(r.Context(), wb, name); err != nil {
		return nil, err
	}
	networks, err := wb.WifiScan(r.Context(), name, r.URL.Query().Get("rescan") == "1")
	if networks == nil {
		networks = []system.WifiNetwork{}
	}
	return networks, err
}

func (s *Service) wifiConnect(r *http.Request) (any, error) {
	wb, err := s.wifi()
	if err != nil {
		return nil, err
	}
	name := r.PathValue("name")
	var req system.WifiRequest
	if err := decode(r, &req); err != nil {
		return nil, err
	}
	if req, err = req.Normalize(); err != nil {
		return nil, err
	}
	if err := s.device(r.Context(), wb, name); err != nil {
		return nil, err
	}
	// Commands continue if joining ends this client's connection. The
	// password stays in this closure: it is not logged or recorded with the
	// trial, which holds only the name of what to delete.
	ctx, cancel := context.WithTimeout(context.Background(), connectTimeout)
	defer cancel()
	return s.trial.start(ctx, change{
		subject: name,
		begin: func(ctx context.Context) (string, system.Backend, system.Undo, error) {
			undo, err := wb.PrepareWifi(ctx, name, req)
			return "Wi-Fi " + req.SSID, s.backend, undo, err
		},
		apply: func(ctx context.Context, _ system.Backend) error { return wb.ConnectWifi(ctx, name, req) },
	}, s.window)
}

// wifiForget deletes a saved network. A deleted connection cannot be
// restored (its password is not kept), so this does not go on trial; it is
// refused for the network a protected radio is using, and while a change is
// on trial.
func (s *Service) wifiForget(r *http.Request) (any, error) {
	wb, err := s.wifi()
	if err != nil {
		return nil, err
	}
	uuid := r.PathValue("uuid")
	if !uuidPattern.MatchString(uuid) {
		return nil, system.ErrNotFound
	}
	saved, err := wb.WifiSaved(r.Context())
	if err != nil {
		return nil, err
	}
	var found *system.WifiSaved
	for i := range saved {
		if saved[i].UUID == uuid {
			found = &saved[i]
		}
	}
	if found == nil {
		return nil, system.ErrNotFound
	}
	if s.trial.status().Trial != nil {
		return nil, &errInUse{"A change is on trial; keep or revert it first."}
	}
	if found.Active {
		devices, err := s.wifiOverview(r)
		if err != nil {
			return nil, err
		}
		for _, d := range devices.(WifiOverview).Devices {
			if d.Protected && d.SSID == found.SSID {
				return nil, &errInUse{d.Name + " is using " + found.SSID + ", which carries this machine's default route or your connection; forgetting it could leave the panel unreachable."}
			}
		}
	}
	ctx, cancel := context.WithTimeout(context.Background(), applyTimeout)
	defer cancel()
	if err := wb.ForgetWifi(ctx, uuid); err != nil {
		return nil, err
	}
	s.logger.Info("wifi network forgotten", "ssid", found.SSID)
	return map[string]string{"forgotten": found.SSID}, nil
}

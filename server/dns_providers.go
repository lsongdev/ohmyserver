package server

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"strings"

	"github.com/lsongdev/ohmyserver/dnsprovider/providers"
	"github.com/lsongdev/ohmyserver/dnsprovider/types"
)

type DNSChallengeTarget struct {
	Value        string
	Label        string
	Kind         string
	ZoneName     string
	LocalZoneID  int64
	ProviderID   int64
	ProviderName string
	ZoneID       string
}

func (s *Server) GetDNSProviderClient(providerID int64) (types.IDNSProvider, error) {
	item, err := s.GetDNSProvider(providerID)
	if err != nil {
		return nil, fmt.Errorf("get DNS provider connection: %w", err)
	}
	var config types.Config
	if err := json.Unmarshal([]byte(item.Config), &config); err != nil {
		return nil, fmt.Errorf("decode DNS provider configuration: %w", err)
	}
	client, err := providers.NewDNSProviderByName(item.Type, config)
	if err != nil {
		return nil, err
	}
	return client, nil
}

func encodeProviderTarget(providerID int64, zoneID string) string {
	return fmt.Sprintf("provider:%d:%s", providerID, base64.RawURLEncoding.EncodeToString([]byte(zoneID)))
}

func parseProviderTarget(value string) (int64, string, error) {
	parts := strings.SplitN(value, ":", 3)
	if len(parts) != 3 || parts[0] != "provider" {
		return 0, "", fmt.Errorf("invalid hosted DNS target")
	}
	id, err := strconv.ParseInt(parts[1], 10, 64)
	if err != nil {
		return 0, "", fmt.Errorf("invalid DNS provider connection")
	}
	zoneID, err := base64.RawURLEncoding.DecodeString(parts[2])
	if err != nil {
		return 0, "", fmt.Errorf("invalid hosted zone")
	}
	return id, string(zoneID), nil
}

func (s *Server) DNSChallengeTargets() ([]DNSChallengeTarget, []string, error) {
	local, err := s.GetAllDNSZones()
	if err != nil {
		return nil, nil, err
	}
	targets := make([]DNSChallengeTarget, 0, len(local))
	for _, zone := range local {
		targets = append(targets, DNSChallengeTarget{
			Value: fmt.Sprintf("local:%d", zone.ID), Label: "Local DNS — " + zone.Name,
			Kind: "local", ZoneName: zone.Name, LocalZoneID: zone.ID,
		})
	}
	connections, err := s.GetAllDNSProviders()
	if err != nil {
		return nil, nil, err
	}
	var warnings []string
	for _, connection := range connections {
		client, err := s.GetDNSProviderClient(connection.ID)
		if err != nil {
			warnings = append(warnings, fmt.Sprintf("%s: %v", connection.Name, err))
			continue
		}
		zones, err := client.ListZones()
		if err != nil {
			warnings = append(warnings, fmt.Sprintf("%s: %v", connection.Name, err))
			continue
		}
		for _, zone := range zones {
			targets = append(targets, DNSChallengeTarget{
				Value: encodeProviderTarget(connection.ID, zone.ID),
				Label: connection.Name + " — " + zone.Name, Kind: "provider",
				ZoneName: zone.Name, ProviderID: connection.ID,
				ProviderName: connection.Name, ZoneID: zone.ID,
			})
		}
	}
	return targets, warnings, nil
}

func (s *Server) DNSProvidersView(w http.ResponseWriter, r *http.Request) {
	items, err := s.GetAllDNSProviders()
	if err != nil {
		s.Error(w, err)
		return
	}
	s.Render(w, "dns/providers", H{"providers": items})
}

func (s *Server) DNSProviderNewHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodGet {
		s.Render(w, "dns/provider_form", H{})
		return
	}
	if err := r.ParseForm(); err != nil {
		s.Error(w, err)
		return
	}
	name, providerType := strings.TrimSpace(r.FormValue("name")), r.FormValue("type")
	if name == "" || (providerType != "cloudflare" && providerType != "dnspod" && providerType != "namecom") {
		s.Render(w, "dns/provider_form", H{"error": "Name and a supported provider type are required.", "name": name, "type": providerType})
		return
	}
	config := types.Config{}
	switch providerType {
	case "cloudflare":
		config["token"] = strings.TrimSpace(r.FormValue("token"))
		config["apiKey"] = strings.TrimSpace(r.FormValue("api_key"))
		config["email"] = strings.TrimSpace(r.FormValue("email"))
	case "dnspod":
		config["token"] = strings.TrimSpace(r.FormValue("token"))
	case "namecom":
		config["user"] = strings.TrimSpace(r.FormValue("user"))
		config["token"] = strings.TrimSpace(r.FormValue("token"))
	}
	switch providerType {
	case "cloudflare":
		if config["token"] == "" && (config["apiKey"] == "" || config["email"] == "") {
			s.Render(w, "dns/provider_form", H{"error": "Provide a Cloudflare API token, or an email and global API key.", "name": name, "type": providerType})
			return
		}
	case "dnspod":
		if config["token"] == "" {
			s.Render(w, "dns/provider_form", H{"error": "DNSPod token is required.", "name": name, "type": providerType})
			return
		}
	case "namecom":
		if config["user"] == "" || config["token"] == "" {
			s.Render(w, "dns/provider_form", H{"error": "Name.com username and token are required.", "name": name, "type": providerType})
			return
		}
	}
	raw, err := json.Marshal(config)
	if err == nil {
		_, err = s.CreateDNSProvider(name, providerType, string(raw))
	}
	if err != nil {
		s.Render(w, "dns/provider_form", H{"error": err.Error(), "name": name, "type": providerType})
		return
	}
	http.Redirect(w, r, "/dns/providers", http.StatusSeeOther)
}

func (s *Server) DNSProviderHandler(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		s.Error(w, err)
		return
	}
	if requestMethod(r) == http.MethodDelete {
		if err := s.DeleteDNSProvider(id); err != nil {
			s.Error(w, err)
			return
		}
		http.Redirect(w, r, "/dns/providers", http.StatusSeeOther)
		return
	}
	item, err := s.GetDNSProvider(id)
	if err != nil {
		s.Error(w, err)
		return
	}
	client, err := s.GetDNSProviderClient(id)
	if err != nil {
		s.Error(w, err)
		return
	}
	zones, err := client.ListZones()
	if err != nil {
		s.Error(w, err)
		return
	}
	s.Render(w, "dns/provider_detail", H{"provider": item, "zones": zones})
}

func (s *Server) DNSProviderZoneHandler(w http.ResponseWriter, r *http.Request) {
	providerID, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		s.Error(w, err)
		return
	}
	zoneIDRaw, err := base64.RawURLEncoding.DecodeString(r.PathValue("zone"))
	if err != nil {
		s.Error(w, fmt.Errorf("invalid hosted zone: %w", err))
		return
	}
	zoneID := string(zoneIDRaw)
	item, err := s.GetDNSProvider(providerID)
	if err != nil {
		s.Error(w, err)
		return
	}
	client, err := s.GetDNSProviderClient(providerID)
	if err != nil {
		s.Error(w, err)
		return
	}
	zoneName := zoneID
	if zones, listErr := client.ListZones(); listErr == nil {
		for _, zone := range zones {
			if zone.ID == zoneID {
				zoneName = zone.Name
				break
			}
		}
	}
	if r.Method == http.MethodDelete {
		recordID := r.URL.Query().Get("record_id")
		if recordID == "" {
			s.Error(w, fmt.Errorf("missing record_id"))
			return
		}
		if err := client.DeleteRecord(zoneID, recordID); err != nil {
			s.Error(w, err)
			return
		}
		http.Redirect(w, r, r.URL.Path, http.StatusSeeOther)
		return
	}
	if r.Method == http.MethodPost {
		if err := r.ParseForm(); err != nil {
			s.Error(w, err)
			return
		}
		method := strings.ToUpper(r.FormValue("_method"))
		record := &types.Record{
			ID: r.FormValue("record_id"), Type: strings.ToUpper(r.FormValue("type")),
			Name: strings.TrimSpace(r.FormValue("name")), Value: strings.TrimSpace(r.FormValue("value")),
			TTL: 300, Comment: strings.TrimSpace(r.FormValue("comment")),
		}
		if ttl, parseErr := strconv.Atoi(r.FormValue("ttl")); parseErr == nil && ttl > 0 {
			record.TTL = ttl
		}
		switch method {
		case http.MethodDelete:
			err = client.DeleteRecord(zoneID, record.ID)
		case http.MethodPut:
			err = client.UpdateRecord(zoneID, record)
		default:
			err = client.AddRecord(zoneID, record)
		}
		if err != nil {
			s.Error(w, err)
			return
		}
		http.Redirect(w, r, r.URL.Path, http.StatusSeeOther)
		return
	}
	records, err := client.ListRecords(zoneID)
	if err != nil {
		s.Error(w, err)
		return
	}
	s.Render(w, "dns/provider_zone", H{
		"provider": item, "zoneID": zoneID, "zoneKey": r.PathValue("zone"),
		"zoneName": zoneName, "records": records,
	})
}

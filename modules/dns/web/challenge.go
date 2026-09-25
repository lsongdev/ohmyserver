package web

import (
	"context"
	"database/sql"
	"encoding/base64"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	dnsfilterzone "github.com/lsongdev/dns-go/zone"
	dnsprovider "github.com/lsongdev/ohmyserver/modules/dns/providers/types"
)

// ChallengeTarget is a DNS module capability exposed to ACME by the app
// composition layer. Provider IDs and hosted-zone IDs stay opaque to ACME.
type ChallengeTarget struct {
	Value        string
	Label        string
	Kind         string
	ZoneName     string
	LocalZoneID  int64
	ProviderID   int64
	ProviderName string
	ZoneID       string
}

func (s *LocalService) ChallengeTargets(ctx context.Context) ([]ChallengeTarget, []string, error) {
	if err := ctx.Err(); err != nil {
		return nil, nil, err
	}
	zones, err := s.repo.Zones(ctx)
	if err != nil {
		return nil, nil, err
	}
	targets := make([]ChallengeTarget, 0, len(zones))
	for _, zone := range zones {
		targets = append(targets, ChallengeTarget{Value: fmt.Sprintf("local:%d", zone.ID), Label: "Local DNS — " + zone.Name, Kind: "local", ZoneName: zone.Name, LocalZoneID: zone.ID})
	}
	connections, err := s.repo.List(ctx)
	if err != nil {
		return nil, nil, err
	}
	var warnings []string
	for _, connection := range connections {
		if err := ctx.Err(); err != nil {
			return nil, warnings, err
		}
		client, err := NewProviderClient(ctx, s.repo, connection.ID)
		if err != nil {
			if ctxErr := ctx.Err(); ctxErr != nil {
				return nil, warnings, ctxErr
			}
			warnings = append(warnings, fmt.Sprintf("%s: %v", connection.Name, err))
			continue
		}
		providerZones, err := client.ListZones(ctx)
		if ctxErr := ctx.Err(); ctxErr != nil {
			return nil, warnings, ctxErr
		}
		if err != nil {
			warnings = append(warnings, fmt.Sprintf("%s: %v", connection.Name, err))
			continue
		}
		for _, zone := range providerZones {
			targets = append(targets, ChallengeTarget{Value: encodeProviderTarget(connection.ID, zone.ID), Label: connection.Name + " — " + zone.Name, Kind: "provider", ZoneName: zone.Name, ProviderID: connection.ID, ProviderName: connection.Name, ZoneID: zone.ID})
		}
	}
	return targets, warnings, nil
}

// PresentTXT creates a challenge TXT record for a previously enumerated
// target and returns an idempotent cleanup closure.
func (s *LocalService) PresentTXT(ctx context.Context, target, name, value string) (func() error, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if strings.TrimSpace(name) == "" || strings.TrimSpace(value) == "" {
		return nil, errors.New("TXT record name and value are required")
	}
	if strings.HasPrefix(target, "local:") {
		zoneID, err := strconv.ParseInt(strings.TrimPrefix(target, "local:"), 10, 64)
		if err != nil || zoneID < 1 {
			return nil, errors.New("invalid local DNS zone")
		}
		zone, err := s.repo.Zone(ctx, zoneID)
		if err != nil {
			return nil, err
		}
		record := &LocalRecord{ZoneID: zone.ID, Name: name, Type: "TXT", TTL: 60, Value: value, Enabled: true}
		if _, err := dnsfilterzone.Parse([]byte("$ORIGIN " + zone.Name + ".\n" + RecordLine(*record) + "\n")); err != nil {
			return nil, fmt.Errorf("invalid challenge TXT record: %w", err)
		}
		if err := s.repo.SaveRecord(ctx, record); err != nil {
			return nil, err
		}
		if err := s.Reload(ctx, false); err != nil {
			cleanupCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
			defer cancel()
			_ = s.repo.DeleteRecord(cleanupCtx, record.ID)
			_ = s.Reload(cleanupCtx, false)
			return nil, err
		}
		return func() error {
			cleanupCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 10*time.Second)
			defer cancel()
			if err := s.repo.DeleteRecord(cleanupCtx, record.ID); err != nil && !errors.Is(err, sql.ErrNoRows) {
				return err
			}
			return s.Reload(cleanupCtx, false)
		}, nil
	}

	providerID, zoneID, err := parseProviderTarget(target)
	if err != nil {
		return nil, err
	}
	connection, err := s.repo.Get(ctx, providerID)
	if err != nil {
		return nil, err
	}
	client, err := NewProviderClient(ctx, s.repo, providerID)
	if err != nil {
		return nil, err
	}
	zones, err := client.ListZones(ctx)
	if ctxErr := ctx.Err(); ctxErr != nil {
		return nil, ctxErr
	}
	if err != nil {
		return nil, err
	}
	found := false
	for _, zone := range zones {
		if zone.ID == zoneID {
			found = true
			break
		}
	}
	if !found {
		return nil, fmt.Errorf("hosted zone is no longer available from %s", connection.Name)
	}
	before, err := client.ListRecords(ctx, zoneID)
	if ctxErr := ctx.Err(); ctxErr != nil {
		return nil, ctxErr
	}
	if err != nil {
		return nil, err
	}
	existing := make(map[string]struct{}, len(before))
	for _, record := range before {
		existing[record.ID] = struct{}{}
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if err := client.AddRecord(ctx, zoneID, &dnsprovider.Record{Type: "TXT", Name: name, Value: value, TTL: 60, Comment: "ACME DNS-01"}); err != nil {
		return nil, err
	}
	after, err := client.ListRecords(ctx, zoneID)
	if ctxErr := ctx.Err(); ctxErr != nil {
		return nil, ctxErr
	}
	if err != nil {
		return nil, err
	}
	recordID := ""
	for _, record := range after {
		_, wasPresent := existing[record.ID]
		recordName := strings.ToLower(strings.TrimSuffix(record.Name, "."))
		if !wasPresent && strings.EqualFold(record.Type, "TXT") && recordName == strings.ToLower(name) && record.Value == value {
			recordID = record.ID
			break
		}
	}
	if recordID == "" {
		return nil, errors.New("DNS provider created the TXT record but did not return its record ID")
	}
	cleanup := func() error {
		cleanupCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 10*time.Second)
		defer cancel()
		return client.DeleteRecord(cleanupCtx, zoneID, recordID)
	}
	if err := ctx.Err(); err != nil {
		_ = cleanup()
		return nil, err
	}
	return cleanup, nil
}

func encodeProviderTarget(providerID int64, zoneID string) string {
	return fmt.Sprintf("provider:%d:%s", providerID, base64.RawURLEncoding.EncodeToString([]byte(zoneID)))
}

func parseProviderTarget(value string) (int64, string, error) {
	parts := strings.SplitN(value, ":", 3)
	if len(parts) != 3 || parts[0] != "provider" {
		return 0, "", errors.New("invalid hosted DNS target")
	}
	id, err := strconv.ParseInt(parts[1], 10, 64)
	if err != nil || id < 1 {
		return 0, "", errors.New("invalid DNS provider connection")
	}
	zoneID, err := base64.RawURLEncoding.DecodeString(parts[2])
	if err != nil {
		return 0, "", errors.New("invalid hosted zone")
	}
	return id, string(zoneID), nil
}

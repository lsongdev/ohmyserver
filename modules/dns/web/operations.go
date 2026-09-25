package web

import (
	"context"
	"fmt"
	"net/url"
	"strings"
	"time"

	dnsfilter "github.com/lsongdev/dns-go/filter"
	dnszone "github.com/lsongdev/dns-go/zone"
	dnsmodule "github.com/lsongdev/ohmyserver/modules/dns"
)

func (m *Module) createZone(ctx context.Context, name, description string, enabled bool) (*dnsmodule.Zone, error) {
	name = normalizeZoneName(name)
	if name == "" || !strings.Contains(name, ".") {
		return nil, fmt.Errorf("a valid DNS zone name is required")
	}
	item, err := m.repo.CreateZone(ctx, name, description, enabled)
	if err != nil {
		return nil, err
	}
	if err = m.service.ReloadDNS(ctx); err != nil {
		cleanupCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
		defer cancel()
		_ = m.repo.DeleteZone(cleanupCtx, item.ID)
		return nil, err
	}
	return item, nil
}
func (m *Module) updateZone(ctx context.Context, id int64, name, description string, enabled bool) error {
	name = normalizeZoneName(name)
	if name == "" || !strings.Contains(name, ".") {
		return fmt.Errorf("a valid DNS zone name is required")
	}
	if err := m.repo.UpdateZone(ctx, id, name, description, enabled); err != nil {
		return err
	}
	return m.service.ReloadDNS(ctx)
}
func (m *Module) deleteZone(ctx context.Context, id int64) error {
	if err := m.repo.DeleteZone(ctx, id); err != nil {
		return err
	}
	return m.service.ReloadDNS(ctx)
}
func (m *Module) saveRecord(ctx context.Context, item *dnsmodule.LocalRecord) error {
	zone, err := m.repo.Zone(ctx, item.ZoneID)
	if err != nil {
		return err
	}
	item.Name = strings.TrimSpace(item.Name)
	if item.Name == "" {
		item.Name = "@"
	}
	item.Type = strings.ToUpper(strings.TrimSpace(item.Type))
	item.Value = strings.TrimSpace(item.Value)
	if item.TTL <= 0 {
		item.TTL = 300
	}
	if _, err := dnszone.Parse([]byte("$ORIGIN " + zone.Name + ".\n" + RecordLine(*item) + "\n")); err != nil {
		return fmt.Errorf("invalid %s record: %w", item.Type, err)
	}
	if err := m.repo.SaveRecord(ctx, item); err != nil {
		return err
	}
	return m.service.ReloadDNS(ctx)
}
func (m *Module) deleteRecord(ctx context.Context, id int64) error {
	if err := m.repo.DeleteRecord(ctx, id); err != nil {
		return err
	}
	return m.service.ReloadDNS(ctx)
}
func normalizeZoneName(value string) string {
	return strings.ToLower(strings.TrimSuffix(strings.TrimSpace(value), "."))
}

func (m *Module) saveSubscription(ctx context.Context, item *dnsmodule.Subscription) error {
	if strings.TrimSpace(item.Name) == "" {
		return fmt.Errorf("subscription name is required")
	}
	if item.Kind != "block" && item.Kind != "allow" {
		return fmt.Errorf("subscription kind must be block or allow")
	}
	parsed, err := url.ParseRequestURI(item.URL)
	if err != nil || (parsed.Scheme != "http" && parsed.Scheme != "https") {
		return fmt.Errorf("subscription URL must use http or https")
	}
	if item.RefreshHours <= 0 {
		item.RefreshHours = 24
	}
	item.Name, item.URL = strings.TrimSpace(item.Name), strings.TrimSpace(item.URL)
	if err := m.repo.SaveSubscription(ctx, item); err != nil {
		return err
	}
	if item.Enabled {
		refreshCtx, cancel := context.WithTimeout(ctx, 50*time.Second)
		err = RefreshSubscription(refreshCtx, m.repo, m.listDir, item)
		cancel()
		if err != nil {
			return err
		}
	}
	return m.service.ReloadDNS(ctx)
}
func (m *Module) subscriptionAction(ctx context.Context, id int64, action string) error {
	item, err := m.repo.Subscription(ctx, id)
	if err != nil {
		return err
	}
	switch action {
	case "toggle":
		item.Enabled = !item.Enabled
		err = m.repo.SaveSubscription(ctx, item)
	case "refresh":
		refreshCtx, cancel := context.WithTimeout(ctx, 50*time.Second)
		err = RefreshSubscription(refreshCtx, m.repo, m.listDir, item)
		cancel()
	case "delete":
		var path string
		path, err = m.repo.DeleteSubscription(ctx, id)
		if err == nil {
			err = RemoveSubscriptionFile(path)
		}
	default:
		return fmt.Errorf("unknown subscription action")
	}
	if err != nil {
		return err
	}
	return m.service.ReloadDNS(ctx)
}
func (m *Module) createRule(ctx context.Context, rule string) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	rule = strings.TrimSpace(rule)
	if rule == "" {
		return fmt.Errorf("filter rule is required")
	}
	filter := dnsfilter.New()
	if err := filter.AddRule(rule); err != nil {
		return err
	}
	if _, err := m.repo.CreateRule(ctx, rule); err != nil {
		return err
	}
	return m.service.ReloadDNS(ctx)
}
func (m *Module) ruleAction(ctx context.Context, id int64, action string) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	var err error
	switch action {
	case "toggle":
		err = m.repo.ToggleRule(ctx, id)
	case "delete":
		err = m.repo.DeleteRule(ctx, id)
	default:
		return fmt.Errorf("unknown rule action")
	}
	if err != nil {
		return err
	}
	return m.service.ReloadDNS(ctx)
}

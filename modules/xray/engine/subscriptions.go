package engine

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/lsongdev/ohmyserver/modules/xray/subscription"
)

type ImportedNode struct {
	ID       string `json:"id"`
	Tag      string `json:"tag"`
	Name     string `json:"name"`
	Missing  bool   `json:"missing"`
	Endpoint string `json:"endpoint,omitempty"`
}
type Subscription struct {
	ID         string              `json:"id"`
	Name       string              `json:"name"`
	URL        string              `json:"url,omitempty"`
	Updated    string              `json:"updated,omitempty"`
	Error      string              `json:"error,omitempty"`
	Nodes      []ImportedNode      `json:"nodes"`
	Candidates []subscription.Node `json:"candidates,omitempty"`
	Warnings   []string            `json:"warnings,omitempty"`
}
type Source struct {
	ID       string `json:"id"`
	Name     string `json:"name"`
	NodeName string `json:"nodeName"`
	Missing  bool   `json:"missing"`
}

func (e *Engine) sources() map[string]Source {
	sources := map[string]Source{}
	for _, s := range e.state.Subscriptions {
		for _, n := range s.Nodes {
			sources[n.Tag] = Source{ID: s.ID, Name: s.Name, NodeName: n.Name, Missing: n.Missing}
		}
	}
	return sources
}
func cloneSubscriptions(s []Subscription) []Subscription {
	b, _ := json.Marshal(s)
	var out []Subscription
	_ = json.Unmarshal(b, &out)
	return out
}
func (e *Engine) Subscriptions() ([]Subscription, uint64) {
	e.mu.Lock()
	defer e.mu.Unlock()
	list := cloneSubscriptions(e.state.Subscriptions)
	if list == nil {
		list = []Subscription{}
	}
	for i := range list {
		list[i].URL = ""
		list[i].Candidates = nil
	}
	return list, e.state.Revision
}
func (e *Engine) Subscription(id string) (Subscription, uint64, error) {
	e.mu.Lock()
	defer e.mu.Unlock()
	for _, s := range cloneSubscriptions(e.state.Subscriptions) {
		if s.ID == id {
			s.URL = ""
			return s, e.state.Revision, nil
		}
	}
	return Subscription{}, e.state.Revision, fmt.Errorf("%w: subscription not found", ErrInvalid)
}
func (e *Engine) SaveSubscription(revision uint64, id, name, address string) (Subscription, error) {
	e.mu.Lock()
	defer e.mu.Unlock()
	if err := e.check(revision); err != nil {
		return Subscription{}, err
	}
	if strings.TrimSpace(name) == "" || len(name) > 200 {
		return Subscription{}, fmt.Errorf("%w: name must contain 1 to 200 characters", ErrInvalid)
	}
	address = strings.TrimSpace(address)
	if address != "" {
		if err := subscription.ValidateURL(address); err != nil {
			return Subscription{}, fmt.Errorf("%w: %v", ErrInvalid, err)
		}
	}
	next := e.state
	next.Subscriptions = cloneSubscriptions(e.state.Subscriptions)
	index := -1
	for i, s := range next.Subscriptions {
		if s.ID == id {
			index = i
		}
	}
	if id != "" && index < 0 {
		return Subscription{}, fmt.Errorf("%w: subscription not found", ErrInvalid)
	}
	if index < 0 {
		if address == "" {
			return Subscription{}, fmt.Errorf("%w: URL is required", ErrInvalid)
		}
		b := make([]byte, 8)
		if _, err := rand.Read(b); err != nil {
			return Subscription{}, err
		}
		next.Subscriptions = append(next.Subscriptions, Subscription{ID: hex.EncodeToString(b), Nodes: []ImportedNode{}})
		index = len(next.Subscriptions) - 1
	}
	s := &next.Subscriptions[index]
	s.Name = strings.TrimSpace(name)
	if address != "" && address != s.URL {
		s.URL = address
		s.Updated = ""
		s.Error = ""
		s.Candidates = nil
		s.Warnings = nil
	}
	if _, err := e.commit(next); err != nil {
		return Subscription{}, err
	}
	result := *s
	result.URL = ""
	return result, nil
}

// Refresh stores a preview only. Import is a separate revision-checked operation.
func (e *Engine) Refresh(ctx context.Context, id string, revision uint64) (Subscription, error) {
	e.mu.Lock()
	if err := e.check(revision); err != nil {
		e.mu.Unlock()
		return Subscription{}, err
	}
	address := ""
	for _, s := range e.state.Subscriptions {
		if s.ID == id {
			address = s.URL
		}
	}
	e.mu.Unlock()
	if address == "" {
		return Subscription{}, fmt.Errorf("%w: subscription not found", ErrInvalid)
	}
	nodes, warnings, fetchErr := subscription.Fetch(ctx, address)
	// Validate each converted node with the embedded core before offering it for import.
	valid := []subscription.Node{}
	for _, node := range nodes {
		raw := json.RawMessage(`{"outbounds":[` + string(node.Outbound) + `]}`)
		if _, err := compile(raw); err != nil {
			warnings = append(warnings, "A node could not be compiled by Xray and was skipped.")
			continue
		}
		valid = append(valid, node)
	}
	if fetchErr == nil && len(valid) == 0 {
		fetchErr = fmt.Errorf("no nodes accepted by Xray")
	}
	e.mu.Lock()
	defer e.mu.Unlock()
	if err := e.check(revision); err != nil {
		return Subscription{}, err
	}
	next := e.state
	next.Subscriptions = cloneSubscriptions(e.state.Subscriptions)
	for i := range next.Subscriptions {
		s := &next.Subscriptions[i]
		if s.ID != id {
			continue
		}
		if fetchErr != nil {
			s.Error = fetchErr.Error()
		} else {
			reconcileNodes(s.Candidates, valid, s.Nodes)
			s.Candidates = valid
			s.Warnings = warnings
			s.Updated = time.Now().UTC().Format(time.RFC3339)
			s.Error = ""
			present := map[string]bool{}
			for _, n := range valid {
				present[n.ID] = true
			}
			for j := range s.Nodes {
				s.Nodes[j].Missing = !present[s.Nodes[j].ID]
			}
		}
		if _, err := e.commit(next); err != nil {
			return Subscription{}, err
		}
		result := *s
		result.URL = ""
		return result, nil
	}
	return Subscription{}, fmt.Errorf("%w: subscription not found", ErrInvalid)
}

func (e *Engine) ImportSubscription(revision uint64, id string, ids []string) (Snapshot, error) {
	e.mu.Lock()
	defer e.mu.Unlock()
	if err := e.check(revision); err != nil {
		return Snapshot{}, err
	}
	if len(ids) == 0 {
		return Snapshot{}, fmt.Errorf("%w: select at least one node", ErrInvalid)
	}
	next := e.state
	next.Subscriptions = cloneSubscriptions(e.state.Subscriptions)
	var s *Subscription
	for i := range next.Subscriptions {
		if next.Subscriptions[i].ID == id {
			s = &next.Subscriptions[i]
		}
	}
	if s == nil {
		return Snapshot{}, fmt.Errorf("%w: subscription not found", ErrInvalid)
	}
	var config map[string]json.RawMessage
	if err := json.Unmarshal(next.Draft, &config); err != nil {
		return Snapshot{}, err
	}
	var outbounds []map[string]json.RawMessage
	if err := json.Unmarshal(config["outbounds"], &outbounds); err != nil && len(config["outbounds"]) != 0 {
		return Snapshot{}, err
	}
	positions := map[string]int{}
	for i, ob := range outbounds {
		var tag string
		_ = json.Unmarshal(ob["tag"], &tag)
		positions[tag] = i
	}
	selected := map[string]bool{}
	for _, id := range ids {
		selected[id] = true
	}
	for _, n := range s.Candidates {
		if !selected[n.ID] {
			continue
		}
		delete(selected, n.ID)
		tag := "sub-" + s.ID + "-" + n.ID[:12]
		known := -1
		for i, imported := range s.Nodes {
			if imported.ID == n.ID {
				known = i
				tag = imported.Tag
			}
		}
		if known < 0 {
			if _, exists := positions[tag]; exists {
				return Snapshot{}, fmt.Errorf("%w: generated tag already exists", ErrInvalid)
			}
			s.Nodes = append(s.Nodes, ImportedNode{ID: n.ID, Tag: tag, Name: n.Name, Endpoint: endpointKey(n)})
		} else {
			s.Nodes[known].Name = n.Name
			s.Nodes[known].Missing = false
		}
		var outbound map[string]json.RawMessage
		_ = json.Unmarshal(n.Outbound, &outbound)
		outbound["tag"], _ = json.Marshal(tag)
		if pos, exists := positions[tag]; exists {
			outbounds[pos] = outbound
		} else {
			positions[tag] = len(outbounds)
			outbounds = append(outbounds, outbound)
		}
	}
	if len(selected) > 0 {
		return Snapshot{}, fmt.Errorf("%w: preview changed; fetch again", ErrInvalid)
	}
	config["outbounds"], _ = json.Marshal(outbounds)
	next.Draft, _ = json.Marshal(config)
	if _, err := compile(next.Draft); err != nil {
		return Snapshot{}, fmt.Errorf("%w: imported configuration could not be compiled", ErrInvalid)
	}
	return e.commit(next)
}

// Removing a subscription detaches its nodes, leaving the saved outbounds intact.
func (e *Engine) DeleteSubscription(revision uint64, id string) (Snapshot, error) {
	e.mu.Lock()
	defer e.mu.Unlock()
	if err := e.check(revision); err != nil {
		return Snapshot{}, err
	}
	next := e.state
	next.Subscriptions = []Subscription{}
	found := false
	for _, s := range e.state.Subscriptions {
		if s.ID == id {
			found = true
		} else {
			next.Subscriptions = append(next.Subscriptions, s)
		}
	}
	if !found {
		return Snapshot{}, fmt.Errorf("%w: subscription not found", ErrInvalid)
	}
	return e.commit(next)
}

// A unique endpoint matches the same node across credential or transport changes.
// Ambiguous endpoints retain their content-based identities.

func endpointKey(n subscription.Node) string {
	var ob struct {
		Settings struct {
			Address string
			Port    uint16
			Servers []struct {
				Address string
				Port    uint16
			}
			Vnext []struct {
				Address string
				Port    uint16
			}
		}
	}
	_ = json.Unmarshal(n.Outbound, &ob)
	address, port := ob.Settings.Address, ob.Settings.Port
	if address == "" && len(ob.Settings.Servers) == 1 {
		address = ob.Settings.Servers[0].Address
		port = ob.Settings.Servers[0].Port
	}
	if address == "" && len(ob.Settings.Vnext) == 1 {
		address = ob.Settings.Vnext[0].Address
		port = ob.Settings.Vnext[0].Port
	}
	if address == "" {
		return ""
	}
	return fmt.Sprintf("%s/%s/%d", n.Protocol, address, port)
}

func reconcileNodes(previous, current []subscription.Node, imported []ImportedNode) {
	old := map[string][]string{}
	counts := map[string]int{}
	for _, n := range previous {
		k := endpointKey(n)
		old[k] = append(old[k], n.ID)
	}
	for _, n := range imported {
		if n.Endpoint == "" {
			continue
		}
		exists := false
		for _, id := range old[n.Endpoint] {
			if id == n.ID {
				exists = true
			}
		}
		if !exists {
			old[n.Endpoint] = append(old[n.Endpoint], n.ID)
		}
	}
	for _, n := range current {
		counts[endpointKey(n)]++
	}
	for i, n := range current {
		k := endpointKey(n)
		if k != "" && len(old[k]) == 1 && counts[k] == 1 {
			current[i].ID = old[k][0]
		}
	}
}

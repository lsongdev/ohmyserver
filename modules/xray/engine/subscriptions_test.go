package engine

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestSubscriptionImportRefreshAndPersistence(t *testing.T) {
	password := "one"
	present := true
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if present {
			fmt.Fprintf(w, "trojan://%s@example.com:443#Node", password)
		} else {
			fmt.Fprint(w, "trojan://other@elsewhere.com:443#Other")
		}
	}))
	defer server.Close()
	dir := t.TempDir()
	e, err := Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer e.Close()
	sub, err := e.SaveSubscription(0, "", "Test", server.URL)
	if err != nil {
		t.Fatal(err)
	}
	sub, err = e.Refresh(context.Background(), sub.ID, e.Snapshot().Revision)
	if err != nil || sub.Error != "" {
		t.Fatalf("refresh %v %s", err, sub.Error)
	}
	originalID := sub.Candidates[0].ID
	snapshot, err := e.ImportSubscription(e.Snapshot().Revision, sub.ID, []string{originalID})
	if err != nil {
		t.Fatal(err)
	}
	var cfg struct {
		Outbounds []struct {
			Tag      string
			Settings struct{ Password string }
		}
	}
	_ = json.Unmarshal(snapshot.Config, &cfg)
	tag := cfg.Outbounds[1].Tag
	password = "two"
	sub, err = e.Refresh(context.Background(), sub.ID, snapshot.Revision)
	if err != nil {
		t.Fatal(err)
	}
	if sub.Candidates[0].ID != originalID {
		t.Fatal("credential change changed node identity")
	}
	snapshot, err = e.ImportSubscription(e.Snapshot().Revision, sub.ID, []string{originalID})
	if err != nil {
		t.Fatal(err)
	}
	_ = json.Unmarshal(snapshot.Config, &cfg)
	if len(cfg.Outbounds) != 2 || cfg.Outbounds[1].Tag != tag || cfg.Outbounds[1].Settings.Password != "two" {
		t.Fatal("update must replace in place preserving tag")
	}
	present = false
	sub, err = e.Refresh(context.Background(), sub.ID, snapshot.Revision)
	if err != nil {
		t.Fatal(err)
	}
	if !sub.Nodes[0].Missing {
		t.Fatal("removed node should be marked missing")
	}
	password = "three"
	present = true
	sub, err = e.Refresh(context.Background(), sub.ID, e.Snapshot().Revision)
	if err != nil || sub.Candidates[0].ID != originalID {
		t.Fatal("returning node must keep identity")
	}
	present = false
	sub, err = e.Refresh(context.Background(), sub.ID, e.Snapshot().Revision)
	if err != nil {
		t.Fatal(err)
	}
	snapshot = e.Snapshot()
	if !strings.Contains(string(snapshot.Config), tag) {
		t.Fatal("removed node must remain")
	}
	e.Close()
	e, err = Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer e.Close()
	saved, revision, err := e.Subscription(sub.ID)
	if err != nil || !saved.Nodes[0].Missing || saved.URL != "" {
		t.Fatal("subscription persistence or secret redaction failed")
	}
	_, err = e.DeleteSubscription(revision, sub.ID)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(e.Snapshot().Config), tag) {
		t.Fatal("detach must retain outbounds")
	}
}

package dns

import (
	"context"
	"strings"
	"testing"
)

// suffixSource covers the names under one suffix and records what it publishes.
type suffixSource struct {
	suffix    string
	published map[string]string
}

func (s *suffixSource) Covers(fqdn string) bool { return strings.HasSuffix(fqdn, s.suffix) }
func (s *suffixSource) Present(_ context.Context, fqdn, value string) error {
	s.published[fqdn] = value
	return nil
}
func (s *suffixSource) CleanUp(_ context.Context, fqdn, _ string) error {
	delete(s.published, fqdn)
	return nil
}

func TestSourcesRouteEachName(t *testing.T) {
	ctx := context.Background()
	local := &suffixSource{suffix: ".home.lan", published: map[string]string{}}
	cloud := &suffixSource{suffix: ".lsong.org", published: map[string]string{}}
	s := Sources{local, cloud}
	if !s.Covers("_acme-challenge.www.lsong.org") || s.Covers("_acme-challenge.example.net") {
		t.Fatal("coverage")
	}
	s.Present(ctx, "_acme-challenge.nas.home.lan", "a")
	s.Present(ctx, "_acme-challenge.www.lsong.org", "b")
	if local.published["_acme-challenge.nas.home.lan"] != "a" || cloud.published["_acme-challenge.www.lsong.org"] != "b" || len(local.published) != 1 {
		t.Fatalf("local %v cloud %v", local.published, cloud.published)
	}
	if err := s.Present(ctx, "_acme-challenge.example.net", "c"); err == nil {
		t.Fatal("an uncovered name was accepted")
	}
	s.CleanUp(ctx, "_acme-challenge.www.lsong.org", "b")
	if len(cloud.published) != 0 {
		t.Fatal("cleanup")
	}
}

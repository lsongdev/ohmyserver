package api

import (
	"context"
	"encoding/json"
	"net"
	"net/http"
	"net/http/httptest"
	"net/netip"
	"strings"
	"testing"
	"time"

	"github.com/lsongdev/ohmyserver/modules/nat/discovery"
	"github.com/lsongdev/ohmyserver/modules/nat/stun"
)

func TestBackgroundCheckLifecycle(t *testing.T) {
	d := discovery.New()
	entered := make(chan struct{}, 1)
	d.Resolve = func(context.Context, string) ([]netip.Addr, error) {
		return []netip.Addr{netip.MustParseAddr("192.0.2.1")}, nil
	}
	d.Exchange = func(ctx context.Context, _ net.PacketConn, _, _ netip.AddrPort, _ uint32, _ time.Duration) (stun.Response, error) {
		entered <- struct{}{}
		<-ctx.Done()
		return stun.Response{}, ctx.Err()
	}
	s := New(context.Background(), Options{Detector: d})
	defer s.Close()
	h := s.Handler()
	request := func(method, path, body string) *httptest.ResponseRecorder {
		w := httptest.NewRecorder()
		h.ServeHTTP(w, httptest.NewRequest(method, path, strings.NewReader(body)))
		return w
	}
	if w := request("POST", "/checks", `{"servers":["bad"]}`); w.Code != 422 {
		t.Fatal(w.Code, w.Body)
	}
	if w := request("POST", "/checks", `{"servers":["test:3478"]} {}`); w.Code != 400 {
		t.Fatal(w.Code, w.Body)
	}
	w := request("POST", "/checks", `{"servers":["test:3478"]}`)
	if w.Code != 202 {
		t.Fatal(w.Code, w.Body)
	}
	var payload struct {
		Data Check `json:"data"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &payload); err != nil {
		t.Fatal(err)
	}
	id := payload.Data.ID
	<-entered
	if w := request("POST", "/checks", `{"servers":["test:3478"]}`); w.Code != 409 {
		t.Fatal(w.Code, w.Body)
	}
	if w := request("GET", "/checks/"+id, ""); w.Code != 200 {
		t.Fatal(w.Code, w.Body)
	}
	if w := request("POST", "/checks/"+id+"/cancel", ""); w.Code != 200 {
		t.Fatal(w.Code, w.Body)
	}
	// Close waits for background activity; completed checks remain inspectable.
	s.Close()
	w = request("GET", "/checks/"+id, "")
	json.Unmarshal(w.Body.Bytes(), &payload)
	if payload.Data.Status != "cancelled" || payload.Data.Finished == nil {
		t.Fatal(w.Body)
	}
	if w := request("GET", "/checks/missing", ""); w.Code != http.StatusNotFound {
		t.Fatal(w.Code)
	}
}

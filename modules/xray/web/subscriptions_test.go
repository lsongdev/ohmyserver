package web

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestSubscriptionAPI(t *testing.T) {
	provider := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusForbidden)
		_, _ = w.Write([]byte("sensitive provider error body"))
	}))
	defer provider.Close()
	m, err := New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer m.Close()
	handler := m.Handler()
	call := func(method, path, body string) *httptest.ResponseRecorder {
		t.Helper()
		w := httptest.NewRecorder()
		handler.ServeHTTP(w, httptest.NewRequest(method, path, bytes.NewBufferString(body)))
		return w
	}
	saved := call("POST", "/subscriptions", `{"revision":0,"name":"Provider","url":"`+provider.URL+`/secret-token"}`)
	if saved.Code != 200 || strings.Contains(saved.Body.String(), "secret-token") {
		t.Fatalf("save or redaction failure: %d", saved.Code)
	}
	var envelope struct{ Data struct{ ID string } }
	_ = json.Unmarshal(saved.Body.Bytes(), &envelope)
	id := envelope.Data.ID
	refresh := call("POST", "/subscriptions/"+id+"/refresh", `{"revision":1}`)
	if refresh.Code != 200 || !strings.Contains(refresh.Body.String(), "HTTP 403") || strings.Contains(refresh.Body.String(), "sensitive") {
		t.Fatalf("fetch errors should be safe and persisted: %d", refresh.Code)
	}
	listing := call("GET", "/subscriptions", "")
	if listing.Code != 200 || strings.Contains(listing.Body.String(), "secret-token") {
		t.Fatal("list exposed URL")
	}
	stale := call("POST", "/subscriptions/"+id+"/delete", `{"revision":1}`)
	if stale.Code != 409 {
		t.Fatal("stale revision must conflict")
	}
}

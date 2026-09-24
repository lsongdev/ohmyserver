package clash

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestClientReadsControllerConfigWithSecret(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/configs" {
			t.Errorf("request path = %q, want /configs", r.URL.Path)
		}
		if got := r.Header.Get("Authorization"); got != "Bearer test-secret" {
			t.Errorf("Authorization = %q, want bearer token", got)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"mode":"rule","mixed-port":7890}`))
	}))
	defer server.Close()

	config, err := NewClient(server.URL, "test-secret").GetConfig(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if config.Mode != "rule" || config.MixedPort != 7890 {
		t.Fatalf("unexpected controller config: %+v", config)
	}
}

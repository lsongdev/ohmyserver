package web

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestSMSRequiresAuthenticatedHost(t *testing.T) {
	module := &Cellular{}
	handler := module.Handler()
	for _, path := range []string{"/sms/list", "/sms/send"} {
		request := httptest.NewRequest(http.MethodPost, path, strings.NewReader("{}"))
		response := httptest.NewRecorder()
		handler.ServeHTTP(response, request)
		if response.Code != http.StatusForbidden || response.Header().Get("Cache-Control") != "no-store" {
			t.Errorf("%s: status=%d headers=%v", path, response.Code, response.Header())
		}
	}
}

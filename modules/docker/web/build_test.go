package web

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

type buildTestService struct {
	Service
	request BuildImageRequest
}

func (s *buildTestService) BuildDockerImage(_ context.Context, request BuildImageRequest, onLog func(string) error) error {
	s.request = request
	return onLog("step one")
}

func TestBuildImageStreamsLogs(t *testing.T) {
	service := &buildTestService{}
	handler := New(service).Handler()
	req := httptest.NewRequest(http.MethodPost, "/images/build", strings.NewReader(`{"name":"demo:latest","source_type":"dockerfile","dockerfile":"FROM scratch"}`))
	req.Header.Set("Content-Type", "application/json")
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, req)
	if response.Code != http.StatusOK || !strings.Contains(response.Body.String(), "event: log") || !strings.Contains(response.Body.String(), "step one") || !strings.Contains(response.Body.String(), "event: done") {
		t.Fatalf("status=%d body=%s", response.Code, response.Body.String())
	}
	if service.request.Name != "demo:latest" || service.request.SourceType != "dockerfile" {
		t.Fatalf("request = %#v", service.request)
	}
}

func TestBuildImageRejectsMissingSourceBeforeStreaming(t *testing.T) {
	service := &buildTestService{}
	handler := New(service).Handler()
	req := httptest.NewRequest(http.MethodPost, "/images/build", strings.NewReader(`{"name":"demo:latest","source_type":"dockerfile"}`))
	req.Header.Set("Content-Type", "application/json")
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, req)
	if response.Code != http.StatusUnprocessableEntity || strings.Contains(response.Header().Get("Content-Type"), "text/event-stream") {
		t.Fatalf("status=%d headers=%v body=%s", response.Code, response.Header(), response.Body.String())
	}
}

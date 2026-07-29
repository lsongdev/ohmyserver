package server

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestUpdateHandlerQueuesFixedUpdateTrigger(t *testing.T) {
	called := 0
	server := &Server{
		scheduleUpdate: func() error {
			called++
			return nil
		},
	}
	request := httptest.NewRequest(http.MethodPost, "/about/update", nil)
	response := httptest.NewRecorder()

	server.UpdateHandler(response, request)

	if called != 1 {
		t.Fatalf("scheduleUpdate called %d times, want 1", called)
	}
	if response.Code != http.StatusSeeOther {
		t.Fatalf("status = %d, want %d", response.Code, http.StatusSeeOther)
	}
}

func TestUpdateHandlerReportsSchedulingFailure(t *testing.T) {
	server := &Server{
		scheduleUpdate: func() error { return errors.New("path unit unavailable") },
	}
	request := httptest.NewRequest(http.MethodPost, "/about/update", nil)
	response := httptest.NewRecorder()

	server.UpdateHandler(response, request)

	if response.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, want %d", response.Code, http.StatusInternalServerError)
	}
}

func TestUpdateHandlerRejectsGet(t *testing.T) {
	request := httptest.NewRequest(http.MethodGet, "/about/update", nil)
	response := httptest.NewRecorder()

	(&Server{}).UpdateHandler(response, request)

	if response.Code != http.StatusMethodNotAllowed {
		t.Fatalf("status = %d, want %d", response.Code, http.StatusMethodNotAllowed)
	}
}

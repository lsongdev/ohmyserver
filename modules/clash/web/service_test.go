package web

import (
	"context"
	"database/sql"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	_ "modernc.org/sqlite"
)

func TestManagerOwnsClashRecordsAndPreservesUnsetSecret(t *testing.T) {
	db, err := sql.Open("sqlite", "file:clash-manager-test?mode=memory&cache=shared")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	db.SetMaxOpenConns(1)
	repository := NewRepository(db)
	if err := repository.Initialize(context.Background()); err != nil {
		t.Fatal(err)
	}
	manager := NewManager(repository)
	secret := "top-secret"
	created, err := manager.CreateClash(context.Background(), Input{Name: "home", APIURL: "http://127.0.0.1:9090", Secret: &secret})
	if err != nil {
		t.Fatal(err)
	}
	if !created.HasSecret || created.ID == 0 {
		t.Fatalf("created DTO did not report the stored secret: %+v", created)
	}
	updated, err := manager.UpdateClash(context.Background(), created.ID, Input{Name: "home-updated", APIURL: "http://127.0.0.1:9091"})
	if err != nil {
		t.Fatal(err)
	}
	if !updated.HasSecret || updated.Name != "home-updated" {
		t.Fatalf("update did not preserve the secret or name: %+v", updated)
	}
	if err := manager.DeleteClash(context.Background(), created.ID); err != nil {
		t.Fatal(err)
	}
	items, err := manager.ListClash(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 0 {
		t.Fatalf("ListClash returned %d records after deletion", len(items))
	}
}

func TestStandaloneAPIUsesTheModuleManager(t *testing.T) {
	db, err := sql.Open("sqlite", "file:clash-standalone-api-test?mode=memory&cache=shared")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	db.SetMaxOpenConns(1)
	repository := NewRepository(db)
	if err := repository.Initialize(context.Background()); err != nil {
		t.Fatal(err)
	}
	handler := New(NewManager(repository)).Handler()
	request := httptest.NewRequest(http.MethodPost, "/servers", strings.NewReader(`{"name":"home","api_url":"http://127.0.0.1:9090"}`))
	request.Header.Set("Content-Type", "application/json")
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	if response.Code != http.StatusCreated || !strings.Contains(response.Body.String(), `"name":"home"`) {
		t.Fatalf("create response: status=%d body=%s", response.Code, response.Body.String())
	}
	list := httptest.NewRecorder()
	handler.ServeHTTP(list, httptest.NewRequest(http.MethodGet, "/servers", nil))
	if list.Code != http.StatusOK || !strings.Contains(list.Body.String(), `"name":"home"`) {
		t.Fatalf("list response: status=%d body=%s", list.Code, list.Body.String())
	}
}

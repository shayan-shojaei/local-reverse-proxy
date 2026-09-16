package api

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/shayan-shojaei/local-reverse-proxy/internal/controller"
	"github.com/shayan-shojaei/local-reverse-proxy/internal/store"
)

type acceptingApplier struct{}

func (acceptingApplier) Apply(context.Context, map[string]any) error { return nil }

func TestRouteAPI_CreateAndList(t *testing.T) {
	handler := testHandler(t)
	body := []byte(`{"hostname":"api","publicMode":"https","upstream":{"scheme":"http","host":"localhost","port":3000,"skipTlsVerify":false}}`)
	request := httptest.NewRequest(http.MethodPost, "/api/v1/routes", bytes.NewReader(body))
	request.Host = "127.0.0.1:7400"
	request.Header.Set("Content-Type", "application/json")
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	if response.Code != http.StatusCreated {
		t.Fatalf("POST status = %d, body = %s", response.Code, response.Body.String())
	}

	request = httptest.NewRequest(http.MethodGet, "/api/v1/routes", nil)
	request.Host = "127.0.0.1:7400"
	response = httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	if response.Code != http.StatusOK {
		t.Fatalf("GET status = %d", response.Code)
	}
	var result struct {
		Data []json.RawMessage `json:"data"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &result); err != nil || len(result.Data) != 1 {
		t.Fatalf("GET body = %s, error = %v", response.Body.String(), err)
	}
}

func TestRouteAPI_ReturnsProblemForInvalidInput(t *testing.T) {
	handler := testHandler(t)
	request := httptest.NewRequest(http.MethodPost, "/api/v1/routes", bytes.NewBufferString(`{"hostname":"bad_name"}`))
	request.Host = "127.0.0.1:7400"
	request.Header.Set("Content-Type", "application/json")
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	if response.Code != http.StatusBadRequest || response.Header().Get("Content-Type") != "application/problem+json" {
		t.Fatalf("status = %d, content-type = %q, body = %s", response.Code, response.Header().Get("Content-Type"), response.Body.String())
	}
}

func TestRouteAPI_RejectsNonLoopbackHost(t *testing.T) {
	handler := testHandler(t)
	request := httptest.NewRequest(http.MethodGet, "/api/v1/routes", nil)
	request.Host = "evil.example"
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	if response.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want 403", response.Code)
	}
}

func testHandler(t *testing.T) http.Handler {
	t.Helper()
	db, err := store.Open(context.Background(), t.TempDir()+"/lrp.db")
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() { db.Close() })
	return NewHandler(controller.NewService(db, acceptingApplier{}))
}

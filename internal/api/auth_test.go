package api

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestAuthExchangesBootstrapTokenForSession(t *testing.T) {
	auth := NewAuth("correct-token")
	handler := auth.Wrap(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		writer.WriteHeader(http.StatusNoContent)
	}))

	exchange := httptest.NewRequest(http.MethodPost, "/api/v1/auth/exchange", strings.NewReader(`{"token":"correct-token"}`))
	exchange.Host = "127.0.0.1:7400"
	exchange.Header.Set("Content-Type", "application/json")
	exchange.Header.Set("Origin", "http://127.0.0.1:7400")
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, exchange)
	if response.Code != http.StatusNoContent || len(response.Result().Cookies()) != 1 {
		t.Fatalf("exchange status = %d, cookies = %#v", response.Code, response.Result().Cookies())
	}

	request := httptest.NewRequest(http.MethodGet, "/api/v1/routes", nil)
	request.Host = "127.0.0.1:7400"
	request.AddCookie(response.Result().Cookies()[0])
	authorized := httptest.NewRecorder()
	handler.ServeHTTP(authorized, request)
	if authorized.Code != http.StatusNoContent {
		t.Fatalf("authorized status = %d", authorized.Code)
	}
}

func TestAuthRejectsMissingAndWrongTokens(t *testing.T) {
	handler := NewAuth("correct-token").Wrap(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		writer.WriteHeader(http.StatusNoContent)
	}))

	request := httptest.NewRequest(http.MethodGet, "/api/v1/routes", nil)
	request.Host = "127.0.0.1:7400"
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	if response.Code != http.StatusUnauthorized {
		t.Fatalf("missing token status = %d", response.Code)
	}

	request = httptest.NewRequest(http.MethodGet, "/api/v1/routes", nil)
	request.Host = "127.0.0.1:7400"
	request.Header.Set("Authorization", "Bearer wrong-token")
	response = httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	if response.Code != http.StatusUnauthorized {
		t.Fatalf("wrong token status = %d", response.Code)
	}
}

func TestAuthRejectsCrossOriginSessionMutation(t *testing.T) {
	auth := NewAuth("correct-token")
	session := auth.newSession()
	handler := auth.Wrap(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		writer.WriteHeader(http.StatusNoContent)
	}))

	request := httptest.NewRequest(http.MethodPost, "/api/v1/routes", strings.NewReader(`{}`))
	request.Host = "127.0.0.1:7400"
	request.Header.Set("Origin", "https://evil.example")
	request.AddCookie(&http.Cookie{Name: sessionCookie, Value: session})
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	if response.Code != http.StatusForbidden {
		t.Fatalf("cross-origin status = %d", response.Code)
	}
}

func TestAuthRejectsMutationFromDifferentLoopbackPort(t *testing.T) {
	auth := NewAuth("correct-token")
	session := auth.newSession()
	handler := auth.Wrap(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		writer.WriteHeader(http.StatusNoContent)
	}))

	request := httptest.NewRequest(http.MethodPost, "/api/v1/routes", strings.NewReader(`{}`))
	request.Host = "127.0.0.1:7400"
	request.Header.Set("Origin", "http://127.0.0.1:9000")
	request.AddCookie(&http.Cookie{Name: sessionCookie, Value: session})
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	if response.Code != http.StatusForbidden {
		t.Fatalf("different-port status = %d", response.Code)
	}
}

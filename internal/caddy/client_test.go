package caddy

import (
	"context"
	"io"
	"net/http"
	"strings"
	"testing"
)

type roundTripFunc func(*http.Request) (*http.Response, error)

func (fn roundTripFunc) RoundTrip(request *http.Request) (*http.Response, error) { return fn(request) }

func TestClientApplyLoadsJSONConfig(t *testing.T) {
	var method, contentType string
	client := NewClient("http://caddy:2019")
	client.client.Transport = roundTripFunc(func(request *http.Request) (*http.Response, error) {
		method = request.Method
		contentType = request.Header.Get("Content-Type")
		return &http.Response{StatusCode: http.StatusOK, Status: "200 OK", Body: io.NopCloser(strings.NewReader(""))}, nil
	})

	if err := client.Apply(context.Background(), map[string]any{"apps": map[string]any{}}); err != nil {
		t.Fatalf("Apply: %v", err)
	}
	if method != http.MethodPost || contentType != "application/json" {
		t.Fatalf("method = %q, content type = %q", method, contentType)
	}
}

func TestClientApplyReturnsCaddyError(t *testing.T) {
	client := NewClient("http://caddy:2019")
	client.client.Transport = roundTripFunc(func(request *http.Request) (*http.Response, error) {
		return &http.Response{StatusCode: http.StatusBadRequest, Status: "400 Bad Request", Body: io.NopCloser(strings.NewReader("invalid route"))}, nil
	})

	err := client.Apply(context.Background(), map[string]any{})
	if err == nil {
		t.Fatal("Apply succeeded, want error")
	}
}

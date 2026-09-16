package domain

import "testing"

func TestNewRouteNormalizesAndValidates(t *testing.T) {
	route, err := NewRoute(RouteInput{
		Hostname:   " API.Dev ",
		PublicMode: PublicHTTPS,
		Upstream:   Upstream{Scheme: "http", Host: "localhost", Port: 3000},
	})
	if err != nil {
		t.Fatalf("NewRoute returned error: %v", err)
	}
	if route.Hostname != "api.dev" {
		t.Fatalf("hostname = %q, want api.dev", route.Hostname)
	}
	if !route.Enabled {
		t.Fatal("new route should be enabled")
	}
}

func TestNewRouteRejectsHostnameOutsideRelativeSyntax(t *testing.T) {
	invalid := []string{"", ".api", "api.", "api_local", "-api", "api..dev", "api.local.test"}
	for _, hostname := range invalid {
		t.Run(hostname, func(t *testing.T) {
			_, err := NewRoute(RouteInput{
				Hostname:   hostname,
				PublicMode: PublicHTTP,
				Upstream:   Upstream{Scheme: "http", Host: "127.0.0.1", Port: 3000},
			})
			if err == nil {
				t.Fatalf("NewRoute(%q) succeeded, want validation error", hostname)
			}
		})
	}
}

func TestNewRouteValidatesUpstream(t *testing.T) {
	tests := []struct {
		name     string
		upstream Upstream
	}{
		{"unsupported scheme", Upstream{Scheme: "ftp", Host: "localhost", Port: 21}},
		{"empty host", Upstream{Scheme: "http", Host: "", Port: 3000}},
		{"zero port", Upstream{Scheme: "http", Host: "localhost", Port: 0}},
		{"large port", Upstream{Scheme: "http", Host: "localhost", Port: 65536}},
		{"skip verify over http", Upstream{Scheme: "http", Host: "localhost", Port: 3000, SkipTLSVerify: true}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := NewRoute(RouteInput{Hostname: "api", PublicMode: PublicHTTP, Upstream: tt.upstream})
			if err == nil {
				t.Fatal("NewRoute succeeded, want validation error")
			}
		})
	}
}

func TestValidateZoneRequiresReservedTestNamespace(t *testing.T) {
	valid := []string{"local.test", "dev.test", "team.dev.test"}
	for _, zone := range valid {
		if err := ValidateZone(zone); err != nil {
			t.Fatalf("ValidateZone(%q): %v", zone, err)
		}
	}
	invalid := []string{"test", "local.internal", ".test", "LOCAL.TEST."}
	for _, zone := range invalid {
		if err := ValidateZone(zone); err == nil {
			t.Fatalf("ValidateZone(%q) succeeded, want error", zone)
		}
	}
}

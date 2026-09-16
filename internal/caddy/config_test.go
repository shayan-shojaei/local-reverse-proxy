package caddy

import (
	"encoding/json"
	"testing"

	"github.com/shayan/local-reverse-proxy/internal/domain"
)

func TestRenderBuildsHTTPAndHTTPSServers(t *testing.T) {
	routes := []domain.Route{
		{ID: "one", Hostname: "web", Enabled: true, PublicMode: domain.PublicHTTPS, Upstream: domain.Upstream{Scheme: "http", Host: "localhost", Port: 3000}},
		{ID: "two", Hostname: "api", Enabled: true, PublicMode: domain.PublicHTTP, Upstream: domain.Upstream{Scheme: "https", Host: "10.0.0.2", Port: 8443, SkipTLSVerify: true}},
		{ID: "three", Hostname: "off", Enabled: false, PublicMode: domain.PublicHTTP, Upstream: domain.Upstream{Scheme: "http", Host: "localhost", Port: 9000}},
	}

	config, err := Render("local.test", routes)
	if err != nil {
		t.Fatalf("Render: %v", err)
	}
	encoded, _ := json.Marshal(config)
	text := string(encoded)
	for _, expected := range []string{"web.local.test", "api.local.test", "host.docker.internal:3000", "10.0.0.2:8443", "insecure_skip_verify"} {
		if !contains(text, expected) {
			t.Errorf("config missing %q: %s", expected, text)
		}
	}
	if contains(text, "off.local.test") {
		t.Fatalf("disabled route present in config: %s", text)
	}
}

func contains(value, fragment string) bool {
	for i := 0; i+len(fragment) <= len(value); i++ {
		if value[i:i+len(fragment)] == fragment {
			return true
		}
	}
	return false
}

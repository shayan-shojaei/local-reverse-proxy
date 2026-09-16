package controller

import (
	"testing"

	"github.com/shayan/local-reverse-proxy/internal/domain"
)

func TestHealthAddressNormalizesHostLoopback(t *testing.T) {
	for _, host := range []string{"localhost", "127.0.0.1", "::1", "[::1]"} {
		address, serverName := healthAddress(domain.Upstream{Host: host, Port: 3000})
		if address != "host.docker.internal:3000" || serverName != "host.docker.internal" {
			t.Fatalf("healthAddress(%q) = %q, %q", host, address, serverName)
		}
	}
}

func TestHealthAddressFormatsIPv6(t *testing.T) {
	address, serverName := healthAddress(domain.Upstream{Host: "2001:db8::1", Port: 8443})
	if address != "[2001:db8::1]:8443" || serverName != "2001:db8::1" {
		t.Fatalf("healthAddress IPv6 = %q, %q", address, serverName)
	}
}

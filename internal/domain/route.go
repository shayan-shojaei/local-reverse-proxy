package domain

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"net"
	"strings"
	"time"
)

type PublicMode string

const (
	PublicHTTP  PublicMode = "http"
	PublicHTTPS PublicMode = "https"
)

type HealthStatus string

const (
	HealthUnknown     HealthStatus = "unknown"
	HealthReachable   HealthStatus = "reachable"
	HealthUnreachable HealthStatus = "unreachable"
)

type Upstream struct {
	Scheme        string `json:"scheme"`
	Host          string `json:"host"`
	Port          int    `json:"port"`
	SkipTLSVerify bool   `json:"skipTlsVerify"`
}

type Route struct {
	ID         string       `json:"id"`
	Hostname   string       `json:"hostname"`
	Enabled    bool         `json:"enabled"`
	PublicMode PublicMode   `json:"publicMode"`
	Upstream   Upstream     `json:"upstream"`
	Revision   int64        `json:"revision"`
	CreatedAt  time.Time    `json:"createdAt"`
	UpdatedAt  time.Time    `json:"updatedAt"`
	Health     HealthStatus `json:"health"`
}

type RouteInput struct {
	Hostname   string     `json:"hostname"`
	PublicMode PublicMode `json:"publicMode"`
	Upstream   Upstream   `json:"upstream"`
}

func NewRoute(input RouteInput) (Route, error) {
	input.Hostname = strings.ToLower(strings.TrimSpace(input.Hostname))
	input.Upstream.Scheme = strings.ToLower(strings.TrimSpace(input.Upstream.Scheme))
	input.Upstream.Host = strings.TrimSpace(input.Upstream.Host)
	if err := ValidateRelativeHostname(input.Hostname); err != nil {
		return Route{}, err
	}
	if input.PublicMode != PublicHTTP && input.PublicMode != PublicHTTPS {
		return Route{}, errors.New("publicMode must be http or https")
	}
	if err := ValidateUpstream(input.Upstream); err != nil {
		return Route{}, err
	}
	id, err := randomID()
	if err != nil {
		return Route{}, fmt.Errorf("generate route id: %w", err)
	}
	now := time.Now().UTC()
	return Route{
		ID: id, Hostname: input.Hostname, Enabled: true, PublicMode: input.PublicMode,
		Upstream: input.Upstream, Revision: 1, CreatedAt: now, UpdatedAt: now, Health: HealthUnknown,
	}, nil
}

func ValidateRelativeHostname(hostname string) error {
	if hostname == "" || strings.HasSuffix(hostname, ".test") {
		return errors.New("hostname must be relative to the configured .test zone")
	}
	if len(hostname) > 253 || !validDNSName(hostname) {
		return errors.New("hostname must contain valid DNS labels")
	}
	return nil
}

func ValidateZone(zone string) error {
	if zone != strings.ToLower(zone) || strings.HasSuffix(zone, ".") || !strings.HasSuffix(zone, ".test") || zone == ".test" {
		return errors.New("zone must be a lowercase domain beneath .test")
	}
	if !validDNSName(zone) {
		return errors.New("zone must contain valid DNS labels")
	}
	return nil
}

func ValidateUpstream(upstream Upstream) error {
	if upstream.Scheme != "http" && upstream.Scheme != "https" {
		return errors.New("upstream scheme must be http or https")
	}
	if upstream.Host == "" || strings.ContainsAny(upstream.Host, "/?#@ ") {
		return errors.New("upstream host must be an IP address or hostname without a URL scheme")
	}
	if net.ParseIP(strings.Trim(upstream.Host, "[]")) == nil && !validDNSName(upstream.Host) {
		return errors.New("upstream host must be a valid IP address or hostname")
	}
	if upstream.Port < 1 || upstream.Port > 65535 {
		return errors.New("upstream port must be between 1 and 65535")
	}
	if upstream.SkipTLSVerify && upstream.Scheme != "https" {
		return errors.New("skipTlsVerify is only valid for HTTPS upstreams")
	}
	return nil
}

func validDNSName(name string) bool {
	for _, label := range strings.Split(name, ".") {
		if label == "" || len(label) > 63 || label[0] == '-' || label[len(label)-1] == '-' {
			return false
		}
		for _, char := range label {
			if (char < 'a' || char > 'z') && (char < '0' || char > '9') && char != '-' {
				return false
			}
		}
	}
	return true
}

func randomID() (string, error) {
	bytes := make([]byte, 16)
	if _, err := rand.Read(bytes); err != nil {
		return "", err
	}
	return hex.EncodeToString(bytes), nil
}

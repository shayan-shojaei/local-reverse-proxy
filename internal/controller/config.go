package controller

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"time"

	"github.com/shayan/local-reverse-proxy/internal/domain"
)

type ImportMode string

const (
	ImportMerge   ImportMode = "merge"
	ImportReplace ImportMode = "replace"
)

type PortableRoute struct {
	Hostname   string            `json:"hostname"`
	Enabled    bool              `json:"enabled"`
	PublicMode domain.PublicMode `json:"publicMode"`
	Upstream   domain.Upstream   `json:"upstream"`
}

type PortableConfig struct {
	Version int             `json:"version"`
	Zone    string          `json:"zone"`
	Routes  []PortableRoute `json:"routes"`
}

type ImportPreview struct {
	Digest     string `json:"digest"`
	TargetZone string `json:"targetZone"`
	Added      int    `json:"added"`
	Updated    int    `json:"updated"`
	Deleted    int    `json:"deleted"`
}

type pendingImport struct {
	baseFingerprint string
	routes          []domain.Route
	expiresAt       time.Time
}

func (s *Service) ExportConfig(ctx context.Context) (PortableConfig, error) {
	routes, err := s.store.ListRoutes(ctx)
	if err != nil {
		return PortableConfig{}, err
	}
	settings, err := s.store.Settings(ctx)
	if err != nil {
		return PortableConfig{}, err
	}
	portable := make([]PortableRoute, 0, len(routes))
	for _, route := range routes {
		portable = append(portable, PortableRoute{Hostname: route.Hostname, Enabled: route.Enabled, PublicMode: route.PublicMode, Upstream: route.Upstream})
	}
	return PortableConfig{Version: 1, Zone: settings.Zone, Routes: portable}, nil
}

func (s *Service) PreviewImport(ctx context.Context, config PortableConfig, mode ImportMode) (ImportPreview, error) {
	if config.Version != 1 {
		return ImportPreview{}, errors.New("config version must be 1")
	}
	if mode != ImportMerge && mode != ImportReplace {
		return ImportPreview{}, errors.New("import mode must be merge or replace")
	}
	if err := domain.ValidateZone(config.Zone); err != nil {
		return ImportPreview{}, fmt.Errorf("exported zone: %w", err)
	}
	current, err := s.store.ListRoutes(ctx)
	if err != nil {
		return ImportPreview{}, err
	}
	next, preview, err := buildImportedRoutes(current, config.Routes, mode)
	if err != nil {
		return ImportPreview{}, err
	}
	settings, err := s.store.Settings(ctx)
	if err != nil {
		return ImportPreview{}, err
	}
	preview.TargetZone = settings.Zone
	preview.Digest, err = importDigest(current, next)
	if err != nil {
		return ImportPreview{}, err
	}
	s.pendingMu.Lock()
	for digest, pending := range s.pendingImports {
		if pending.expiresAt.Before(time.Now()) {
			delete(s.pendingImports, digest)
		}
	}
	s.pendingImports[preview.Digest] = pendingImport{baseFingerprint: routeFingerprint(current), routes: next, expiresAt: time.Now().Add(10 * time.Minute)}
	s.pendingMu.Unlock()
	return preview, nil
}

func (s *Service) ApplyImport(ctx context.Context, digest string) error {
	s.pendingMu.Lock()
	pending, ok := s.pendingImports[digest]
	delete(s.pendingImports, digest)
	s.pendingMu.Unlock()
	if !ok || pending.expiresAt.Before(time.Now()) {
		return errors.New("import preview is missing or expired")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	current, err := s.store.ListRoutes(ctx)
	if err != nil {
		return err
	}
	if routeFingerprint(current) != pending.baseFingerprint {
		return errors.New("routes changed after preview; preview the import again")
	}
	if err := s.apply(ctx, pending.routes); err != nil {
		return err
	}
	if err := s.store.ReplaceRoutes(ctx, pending.routes); err != nil {
		_ = s.apply(ctx, current)
		return err
	}
	return nil
}

func buildImportedRoutes(current []domain.Route, imported []PortableRoute, mode ImportMode) ([]domain.Route, ImportPreview, error) {
	existing := make(map[string]domain.Route, len(current))
	for _, route := range current {
		existing[route.Hostname] = route
	}
	nextByHost := make(map[string]domain.Route)
	if mode == ImportMerge {
		for _, route := range current {
			nextByHost[route.Hostname] = route
		}
	}
	preview := ImportPreview{}
	seen := make(map[string]bool)
	for _, portable := range imported {
		validated, err := domain.NewRoute(domain.RouteInput{Hostname: portable.Hostname, PublicMode: portable.PublicMode, Upstream: portable.Upstream})
		if err != nil {
			return nil, ImportPreview{}, fmt.Errorf("route %q: %w", portable.Hostname, err)
		}
		if seen[validated.Hostname] {
			return nil, ImportPreview{}, fmt.Errorf("duplicate imported hostname %q", validated.Hostname)
		}
		seen[validated.Hostname] = true
		validated.Enabled = portable.Enabled
		if previous, ok := existing[validated.Hostname]; ok {
			validated.ID, validated.CreatedAt = previous.ID, previous.CreatedAt
			validated.Revision = previous.Revision + 1
			if !samePortable(previous, portable) {
				preview.Updated++
			} else {
				validated.Revision, validated.UpdatedAt = previous.Revision, previous.UpdatedAt
			}
		} else {
			preview.Added++
		}
		nextByHost[validated.Hostname] = validated
	}
	if mode == ImportReplace {
		for hostname := range existing {
			if !seen[hostname] {
				preview.Deleted++
			}
		}
	}
	next := make([]domain.Route, 0, len(nextByHost))
	for _, route := range nextByHost {
		next = append(next, route)
	}
	sort.Slice(next, func(left, right int) bool { return next[left].Hostname < next[right].Hostname })
	return next, preview, nil
}

func samePortable(route domain.Route, portable PortableRoute) bool {
	return route.Hostname == portable.Hostname && route.Enabled == portable.Enabled && route.PublicMode == portable.PublicMode && route.Upstream == portable.Upstream
}

func routeFingerprint(routes []domain.Route) string {
	encoded, _ := json.Marshal(routes)
	sum := sha256.Sum256(encoded)
	return hex.EncodeToString(sum[:])
}

func importDigest(current, next []domain.Route) (string, error) {
	nonce := make([]byte, 16)
	if _, err := rand.Read(nonce); err != nil {
		return "", err
	}
	hash := sha256.New()
	hash.Write([]byte(routeFingerprint(current)))
	hash.Write([]byte(routeFingerprint(next)))
	hash.Write(nonce)
	return hex.EncodeToString(hash.Sum(nil)), nil
}

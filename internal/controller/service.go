package controller

import (
	"context"
	"fmt"
	"sync"

	"github.com/shayan-shojaei/local-reverse-proxy/internal/caddy"
	"github.com/shayan-shojaei/local-reverse-proxy/internal/domain"
	"github.com/shayan-shojaei/local-reverse-proxy/internal/store"
)

type Applier interface {
	Apply(context.Context, map[string]any) error
}

type Service struct {
	store          *store.Store
	applier        Applier
	mu             sync.Mutex
	healthMu       sync.RWMutex
	health         map[string]domain.HealthStatus
	pendingMu      sync.Mutex
	pendingImports map[string]pendingImport
}

type UpdateRouteInput struct {
	RouteInput domain.RouteInput `json:"route"`
	Enabled    bool              `json:"enabled"`
	Revision   int64             `json:"revision"`
}

func NewService(store *store.Store, applier Applier) *Service {
	return &Service{store: store, applier: applier, health: make(map[string]domain.HealthStatus), pendingImports: make(map[string]pendingImport)}
}

func (s *Service) ListRoutes(ctx context.Context) ([]domain.Route, error) {
	routes, err := s.store.ListRoutes(ctx)
	if err != nil {
		return nil, err
	}
	s.healthMu.RLock()
	defer s.healthMu.RUnlock()
	for index := range routes {
		if health, ok := s.health[routes[index].ID]; ok {
			routes[index].Health = health
		}
	}
	return routes, nil
}

func (s *Service) Settings(ctx context.Context) (store.Settings, error) {
	return s.store.Settings(ctx)
}

func (s *Service) CreateRoute(ctx context.Context, input domain.RouteInput) (domain.Route, error) {
	route, err := domain.NewRoute(input)
	if err != nil {
		return domain.Route{}, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	current, err := s.store.ListRoutes(ctx)
	if err != nil {
		return domain.Route{}, err
	}
	for _, existing := range current {
		if existing.Hostname == route.Hostname {
			return domain.Route{}, store.ErrHostnameConflict
		}
	}
	if err := s.apply(ctx, append(current, route)); err != nil {
		return domain.Route{}, err
	}
	if err := s.store.CreateRoute(ctx, route); err != nil {
		_ = s.apply(ctx, current)
		return domain.Route{}, err
	}
	return route, nil
}

func (s *Service) UpdateRoute(ctx context.Context, id string, input UpdateRouteInput) (domain.Route, error) {
	validated, err := domain.NewRoute(input.RouteInput)
	if err != nil {
		return domain.Route{}, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	current, err := s.store.ListRoutes(ctx)
	if err != nil {
		return domain.Route{}, err
	}
	var original domain.Route
	found := false
	for _, candidate := range current {
		if candidate.ID == id {
			original, found = candidate, true
		} else if candidate.Hostname == validated.Hostname {
			return domain.Route{}, store.ErrHostnameConflict
		}
	}
	if !found {
		return domain.Route{}, store.ErrNotFound
	}
	if original.Revision != input.Revision {
		return domain.Route{}, store.ErrRevisionConflict
	}
	validated.ID = original.ID
	validated.Enabled = input.Enabled
	validated.CreatedAt = original.CreatedAt
	validated.Revision = original.Revision
	next := replaceRoute(current, validated)
	if err := s.apply(ctx, next); err != nil {
		return domain.Route{}, err
	}
	updated, err := s.store.UpdateRoute(ctx, validated, input.Revision)
	if err != nil {
		_ = s.apply(ctx, current)
		return domain.Route{}, err
	}
	return updated, nil
}

func (s *Service) DeleteRoute(ctx context.Context, id string, revision int64) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	current, err := s.store.ListRoutes(ctx)
	if err != nil {
		return err
	}
	next := make([]domain.Route, 0, len(current))
	found := false
	for _, route := range current {
		if route.ID == id {
			found = true
			if route.Revision != revision {
				return store.ErrRevisionConflict
			}
			continue
		}
		next = append(next, route)
	}
	if !found {
		return store.ErrNotFound
	}
	if err := s.apply(ctx, next); err != nil {
		return err
	}
	if err := s.store.DeleteRoute(ctx, id, revision); err != nil {
		_ = s.apply(ctx, current)
		return err
	}
	s.healthMu.Lock()
	delete(s.health, id)
	s.healthMu.Unlock()
	return nil
}

func (s *Service) Reconcile(ctx context.Context) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	routes, err := s.store.ListRoutes(ctx)
	if err != nil {
		return err
	}
	return s.apply(ctx, routes)
}

func (s *Service) apply(ctx context.Context, routes []domain.Route) error {
	settings, err := s.store.Settings(ctx)
	if err != nil {
		return err
	}
	config, err := caddy.Render(settings.Zone, routes)
	if err != nil {
		return err
	}
	if err := s.applier.Apply(ctx, config); err != nil {
		return fmt.Errorf("apply caddy configuration: %w", err)
	}
	return nil
}

func replaceRoute(routes []domain.Route, replacement domain.Route) []domain.Route {
	result := make([]domain.Route, len(routes))
	copy(result, routes)
	for index := range result {
		if result[index].ID == replacement.ID {
			result[index] = replacement
			break
		}
	}
	return result
}

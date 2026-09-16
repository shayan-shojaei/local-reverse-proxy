package controller

import (
	"context"
	"errors"
	"testing"

	"github.com/shayan-shojaei/local-reverse-proxy/internal/domain"
	"github.com/shayan-shojaei/local-reverse-proxy/internal/store"
)

type recordingApplier struct {
	calls int
	fail  bool
}

func (a *recordingApplier) Apply(_ context.Context, _ map[string]any) error {
	a.calls++
	if a.fail {
		return errors.New("caddy rejected config")
	}
	return nil
}

func TestCreateRouteAppliesBeforePersisting(t *testing.T) {
	ctx := context.Background()
	db, err := store.Open(ctx, t.TempDir()+"/lrp.db")
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() { db.Close() })
	applier := &recordingApplier{}
	service := NewService(db, applier)

	route, err := service.CreateRoute(ctx, domain.RouteInput{
		Hostname: "api", PublicMode: domain.PublicHTTPS,
		Upstream: domain.Upstream{Scheme: "http", Host: "localhost", Port: 3000},
	})
	if err != nil {
		t.Fatalf("CreateRoute: %v", err)
	}
	if applier.calls != 1 || route.Hostname != "api" {
		t.Fatalf("calls = %d, route = %#v", applier.calls, route)
	}

	routes, _ := db.ListRoutes(ctx)
	if len(routes) != 1 {
		t.Fatalf("persisted routes = %d, want 1", len(routes))
	}
}

func TestCreateRouteDoesNotPersistRejectedConfig(t *testing.T) {
	ctx := context.Background()
	db, err := store.Open(ctx, t.TempDir()+"/lrp.db")
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() { db.Close() })
	service := NewService(db, &recordingApplier{fail: true})

	_, err = service.CreateRoute(ctx, domain.RouteInput{
		Hostname: "api", PublicMode: domain.PublicHTTP,
		Upstream: domain.Upstream{Scheme: "http", Host: "localhost", Port: 3000},
	})
	if err == nil {
		t.Fatal("CreateRoute succeeded, want apply error")
	}
	routes, _ := db.ListRoutes(ctx)
	if len(routes) != 0 {
		t.Fatalf("persisted routes = %d, want 0", len(routes))
	}
}

package store

import (
	"context"
	"errors"
	"testing"

	"github.com/shayan-shojaei/local-reverse-proxy/internal/domain"
)

func TestRouteLifecycleAndRevisionChecks(t *testing.T) {
	ctx := context.Background()
	db, err := Open(ctx, t.TempDir()+"/lrp.db")
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() { db.Close() })

	route, err := domain.NewRoute(domain.RouteInput{
		Hostname: "api", PublicMode: domain.PublicHTTPS,
		Upstream: domain.Upstream{Scheme: "http", Host: "localhost", Port: 3000},
	})
	if err != nil {
		t.Fatalf("NewRoute: %v", err)
	}
	if err := db.CreateRoute(ctx, route); err != nil {
		t.Fatalf("CreateRoute: %v", err)
	}

	routes, err := db.ListRoutes(ctx)
	if err != nil || len(routes) != 1 || routes[0].Hostname != "api" {
		t.Fatalf("ListRoutes = %#v, %v", routes, err)
	}

	route.Upstream.Port = 4000
	updated, err := db.UpdateRoute(ctx, route, 1)
	if err != nil {
		t.Fatalf("UpdateRoute: %v", err)
	}
	if updated.Revision != 2 || updated.Upstream.Port != 4000 {
		t.Fatalf("updated route = %#v", updated)
	}
	if _, err := db.UpdateRoute(ctx, updated, 1); !errors.Is(err, ErrRevisionConflict) {
		t.Fatalf("stale UpdateRoute error = %v, want ErrRevisionConflict", err)
	}
	if err := db.DeleteRoute(ctx, route.ID, 2); err != nil {
		t.Fatalf("DeleteRoute: %v", err)
	}
	routes, err = db.ListRoutes(ctx)
	if err != nil || len(routes) != 0 {
		t.Fatalf("routes after delete = %#v, %v", routes, err)
	}
}

func TestCreateRouteRejectsDuplicateHostname(t *testing.T) {
	ctx := context.Background()
	db, err := Open(ctx, t.TempDir()+"/lrp.db")
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() { db.Close() })

	input := domain.RouteInput{
		Hostname: "api", PublicMode: domain.PublicHTTP,
		Upstream: domain.Upstream{Scheme: "http", Host: "localhost", Port: 3000},
	}
	first, _ := domain.NewRoute(input)
	second, _ := domain.NewRoute(input)
	if err := db.CreateRoute(ctx, first); err != nil {
		t.Fatalf("first CreateRoute: %v", err)
	}
	if err := db.CreateRoute(ctx, second); !errors.Is(err, ErrHostnameConflict) {
		t.Fatalf("second CreateRoute error = %v, want ErrHostnameConflict", err)
	}
}

func TestSettingsDefaultAndUpdateZone(t *testing.T) {
	ctx := context.Background()
	db, err := Open(ctx, t.TempDir()+"/lrp.db")
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() { db.Close() })

	settings, err := db.Settings(ctx)
	if err != nil || settings.Zone != "local.test" {
		t.Fatalf("Settings = %#v, %v", settings, err)
	}
	if err := db.SetZone(ctx, "dev.test"); err != nil {
		t.Fatalf("SetZone: %v", err)
	}
	settings, _ = db.Settings(ctx)
	if settings.Zone != "dev.test" {
		t.Fatalf("zone = %q, want dev.test", settings.Zone)
	}
}

func TestReplaceRoutesAtomicallyReplacesCollection(t *testing.T) {
	ctx := context.Background()
	db, err := Open(ctx, t.TempDir()+"/lrp.db")
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() { db.Close() })
	oldRoute, _ := domain.NewRoute(domain.RouteInput{Hostname: "old", PublicMode: domain.PublicHTTP, Upstream: domain.Upstream{Scheme: "http", Host: "localhost", Port: 3000}})
	newRoute, _ := domain.NewRoute(domain.RouteInput{Hostname: "new", PublicMode: domain.PublicHTTPS, Upstream: domain.Upstream{Scheme: "http", Host: "localhost", Port: 4000}})
	if err := db.CreateRoute(ctx, oldRoute); err != nil {
		t.Fatalf("CreateRoute: %v", err)
	}
	if err := db.ReplaceRoutes(ctx, []domain.Route{newRoute}); err != nil {
		t.Fatalf("ReplaceRoutes: %v", err)
	}
	routes, err := db.ListRoutes(ctx)
	if err != nil || len(routes) != 1 || routes[0].Hostname != "new" {
		t.Fatalf("routes = %#v, error = %v", routes, err)
	}
}

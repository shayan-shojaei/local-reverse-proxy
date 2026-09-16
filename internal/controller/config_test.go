package controller

import (
	"context"
	"testing"

	"github.com/shayan-shojaei/local-reverse-proxy/internal/domain"
	"github.com/shayan-shojaei/local-reverse-proxy/internal/store"
)

func TestPreviewAndApplyImportMergesByHostname(t *testing.T) {
	ctx := context.Background()
	db, err := store.Open(ctx, t.TempDir()+"/lrp.db")
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() { db.Close() })
	service := NewService(db, &recordingApplier{})
	_, err = service.CreateRoute(ctx, domain.RouteInput{Hostname: "api", PublicMode: domain.PublicHTTP, Upstream: domain.Upstream{Scheme: "http", Host: "localhost", Port: 3000}})
	if err != nil {
		t.Fatalf("CreateRoute: %v", err)
	}

	preview, err := service.PreviewImport(ctx, PortableConfig{Version: 1, Zone: "other.test", Routes: []PortableRoute{
		{Hostname: "api", Enabled: false, PublicMode: domain.PublicHTTPS, Upstream: domain.Upstream{Scheme: "http", Host: "localhost", Port: 4000}},
		{Hostname: "web", Enabled: true, PublicMode: domain.PublicHTTP, Upstream: domain.Upstream{Scheme: "http", Host: "localhost", Port: 5000}},
	}}, ImportMerge)
	if err != nil {
		t.Fatalf("PreviewImport: %v", err)
	}
	if preview.Added != 1 || preview.Updated != 1 || preview.Deleted != 0 || preview.Digest == "" {
		t.Fatalf("preview = %#v", preview)
	}
	if err := service.ApplyImport(ctx, preview.Digest); err != nil {
		t.Fatalf("ApplyImport: %v", err)
	}
	routes, _ := service.ListRoutes(ctx)
	if len(routes) != 2 || routes[0].Hostname != "api" || routes[0].Upstream.Port != 4000 || routes[0].Enabled {
		t.Fatalf("routes after import = %#v", routes)
	}
}

func TestReplaceImportReportsDeletedRoutes(t *testing.T) {
	ctx := context.Background()
	db, _ := store.Open(ctx, t.TempDir()+"/lrp.db")
	t.Cleanup(func() { db.Close() })
	service := NewService(db, &recordingApplier{})
	_, _ = service.CreateRoute(ctx, domain.RouteInput{Hostname: "old", PublicMode: domain.PublicHTTP, Upstream: domain.Upstream{Scheme: "http", Host: "localhost", Port: 3000}})
	preview, err := service.PreviewImport(ctx, PortableConfig{Version: 1, Zone: "local.test", Routes: []PortableRoute{}}, ImportReplace)
	if err != nil {
		t.Fatalf("PreviewImport: %v", err)
	}
	if preview.Deleted != 1 {
		t.Fatalf("Deleted = %d, want 1", preview.Deleted)
	}
}

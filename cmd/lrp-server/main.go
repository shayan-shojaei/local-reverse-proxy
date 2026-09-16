package main

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"

	"github.com/shayan/local-reverse-proxy/internal/api"
	"github.com/shayan/local-reverse-proxy/internal/caddy"
	"github.com/shayan/local-reverse-proxy/internal/controller"
	"github.com/shayan/local-reverse-proxy/internal/store"
)

func main() {
	if err := run(); err != nil {
		slog.Error("controller stopped", "error", err)
		os.Exit(1)
	}
}

func run() error {
	dataDir := env("LRP_DATA_DIR", "./data")
	if err := os.MkdirAll(dataDir, 0o700); err != nil {
		return err
	}
	db, err := store.Open(context.Background(), filepath.Join(dataDir, "lrp.db"))
	if err != nil {
		return err
	}
	defer db.Close()

	service := controller.NewService(db, caddy.NewClient(env("LRP_CADDY_ADMIN_URL", "http://127.0.0.1:2019")))
	if err := service.Reconcile(context.Background()); err != nil {
		slog.Warn("initial Caddy reconciliation failed; dashboard remains available", "error", err)
	}

	apiHandler := api.NewHandler(service)
	staticDir := env("LRP_WEB_DIR", "./web/dist")
	handler := spaHandler(apiHandler, staticDir)
	server := &http.Server{
		Addr:              env("LRP_LISTEN", "127.0.0.1:7400"),
		Handler:           handler,
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       15 * time.Second,
		WriteTimeout:      30 * time.Second,
		IdleTimeout:       60 * time.Second,
	}

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()
	go func() {
		<-ctx.Done()
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		_ = server.Shutdown(shutdownCtx)
	}()

	slog.Info("dashboard listening", "address", server.Addr)
	err = server.ListenAndServe()
	if errors.Is(err, http.ErrServerClosed) {
		return nil
	}
	return err
}

func spaHandler(apiHandler http.Handler, staticDir string) http.Handler {
	files := http.FileServer(http.Dir(staticDir))
	return http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if len(request.URL.Path) >= 8 && request.URL.Path[:8] == "/api/v1/" {
			apiHandler.ServeHTTP(writer, request)
			return
		}
		path := filepath.Join(staticDir, filepath.Clean(request.URL.Path))
		if info, err := os.Stat(path); err == nil && !info.IsDir() {
			files.ServeHTTP(writer, request)
			return
		}
		http.ServeFile(writer, request, filepath.Join(staticDir, "index.html"))
	})
}

func env(name, fallback string) string {
	if value := os.Getenv(name); value != "" {
		return value
	}
	return fallback
}

package controller

import (
	"context"
	"crypto/tls"
	"net"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/shayan-shojaei/local-reverse-proxy/internal/domain"
)

func (s *Service) StartHealthChecks(ctx context.Context, interval time.Duration) {
	if interval <= 0 {
		interval = 15 * time.Second
	}
	go func() {
		s.checkAllRoutes(ctx)
		ticker := time.NewTicker(interval)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				s.checkAllRoutes(ctx)
			}
		}
	}()
}

func (s *Service) checkAllRoutes(ctx context.Context) {
	routes, err := s.store.ListRoutes(ctx)
	if err != nil {
		return
	}
	var wait sync.WaitGroup
	limit := make(chan struct{}, 8)
	for _, route := range routes {
		if !route.Enabled {
			s.setHealth(route.ID, domain.HealthUnknown)
			continue
		}
		wait.Add(1)
		go func(route domain.Route) {
			defer wait.Done()
			select {
			case limit <- struct{}{}:
				defer func() { <-limit }()
			case <-ctx.Done():
				return
			}
			s.setHealth(route.ID, probeRoute(ctx, route))
		}(route)
	}
	wait.Wait()
}

func probeRoute(ctx context.Context, route domain.Route) domain.HealthStatus {
	address, serverName := healthAddress(route.Upstream)
	dialer := &net.Dialer{Timeout: 3 * time.Second}
	var connection net.Conn
	var err error
	if route.Upstream.Scheme == "https" {
		tlsDialer := &tls.Dialer{NetDialer: dialer, Config: &tls.Config{
			MinVersion: tls.VersionTLS12, ServerName: serverName, InsecureSkipVerify: route.Upstream.SkipTLSVerify, //nolint:gosec -- explicit per-route local development option
		}}
		connection, err = tlsDialer.DialContext(ctx, "tcp", address)
	} else {
		connection, err = dialer.DialContext(ctx, "tcp", address)
	}
	if err != nil {
		return domain.HealthUnreachable
	}
	_ = connection.Close()
	return domain.HealthReachable
}

func healthAddress(upstream domain.Upstream) (string, string) {
	host := strings.Trim(upstream.Host, "[]")
	if host == "localhost" || host == "127.0.0.1" || host == "::1" {
		host = "host.docker.internal"
	}
	return net.JoinHostPort(host, strconv.Itoa(upstream.Port)), host
}

func (s *Service) setHealth(id string, health domain.HealthStatus) {
	s.healthMu.Lock()
	s.health[id] = health
	s.healthMu.Unlock()
}

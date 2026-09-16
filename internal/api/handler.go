package api

import (
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/http"
	"strconv"
	"strings"

	"github.com/shayan/local-reverse-proxy/internal/controller"
	"github.com/shayan/local-reverse-proxy/internal/domain"
	"github.com/shayan/local-reverse-proxy/internal/store"
)

type Handler struct {
	service *controller.Service
}

type problem struct {
	Type   string `json:"type"`
	Title  string `json:"title"`
	Status int    `json:"status"`
	Detail string `json:"detail"`
}

func NewHandler(service *controller.Service, authOptions ...*Auth) http.Handler {
	handler := &Handler{service: service}
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/v1/routes", handler.listRoutes)
	mux.HandleFunc("POST /api/v1/routes", handler.createRoute)
	mux.HandleFunc("PATCH /api/v1/routes/{id}", handler.updateRoute)
	mux.HandleFunc("DELETE /api/v1/routes/{id}", handler.deleteRoute)
	mux.HandleFunc("GET /api/v1/status", handler.status)
	mux.HandleFunc("GET /api/v1/config/export", handler.exportConfig)
	mux.HandleFunc("POST /api/v1/config/import/preview", handler.previewImport)
	mux.HandleFunc("POST /api/v1/config/import/apply", handler.applyImport)
	var root http.Handler = mux
	if len(authOptions) > 0 && authOptions[0] != nil {
		root = authOptions[0].Wrap(root)
	}
	return securityHeaders(loopbackOnly(root))
}

func (h *Handler) exportConfig(writer http.ResponseWriter, request *http.Request) {
	config, err := h.service.ExportConfig(request.Context())
	if err != nil {
		writeError(writer, err)
		return
	}
	writeJSON(writer, http.StatusOK, map[string]any{"data": config})
}

func (h *Handler) previewImport(writer http.ResponseWriter, request *http.Request) {
	var input struct {
		Mode   controller.ImportMode     `json:"mode"`
		Config controller.PortableConfig `json:"config"`
	}
	if err := decodeJSON(writer, request, &input); err != nil {
		writeProblem(writer, http.StatusBadRequest, "invalid_request", err.Error())
		return
	}
	preview, err := h.service.PreviewImport(request.Context(), input.Config, input.Mode)
	if err != nil {
		writeError(writer, err)
		return
	}
	writeJSON(writer, http.StatusOK, map[string]any{"data": preview})
}

func (h *Handler) applyImport(writer http.ResponseWriter, request *http.Request) {
	var input struct {
		Digest string `json:"digest"`
	}
	if err := decodeJSON(writer, request, &input); err != nil {
		writeProblem(writer, http.StatusBadRequest, "invalid_request", err.Error())
		return
	}
	if err := h.service.ApplyImport(request.Context(), input.Digest); err != nil {
		writeError(writer, err)
		return
	}
	writer.WriteHeader(http.StatusNoContent)
}

func (h *Handler) listRoutes(writer http.ResponseWriter, request *http.Request) {
	routes, err := h.service.ListRoutes(request.Context())
	if err != nil {
		writeError(writer, err)
		return
	}
	writeJSON(writer, http.StatusOK, map[string]any{"data": routes})
}

func (h *Handler) createRoute(writer http.ResponseWriter, request *http.Request) {
	var input domain.RouteInput
	if err := decodeJSON(writer, request, &input); err != nil {
		writeProblem(writer, http.StatusBadRequest, "invalid_request", err.Error())
		return
	}
	route, err := h.service.CreateRoute(request.Context(), input)
	if err != nil {
		writeError(writer, err)
		return
	}
	writer.Header().Set("Location", "/api/v1/routes/"+route.ID)
	writeJSON(writer, http.StatusCreated, map[string]any{"data": route})
}

func (h *Handler) updateRoute(writer http.ResponseWriter, request *http.Request) {
	var input struct {
		Hostname   string            `json:"hostname"`
		Enabled    bool              `json:"enabled"`
		PublicMode domain.PublicMode `json:"publicMode"`
		Upstream   domain.Upstream   `json:"upstream"`
		Revision   int64             `json:"revision"`
	}
	if err := decodeJSON(writer, request, &input); err != nil {
		writeProblem(writer, http.StatusBadRequest, "invalid_request", err.Error())
		return
	}
	route, err := h.service.UpdateRoute(request.Context(), request.PathValue("id"), controller.UpdateRouteInput{
		RouteInput: domain.RouteInput{Hostname: input.Hostname, PublicMode: input.PublicMode, Upstream: input.Upstream},
		Enabled:    input.Enabled, Revision: input.Revision,
	})
	if err != nil {
		writeError(writer, err)
		return
	}
	writeJSON(writer, http.StatusOK, map[string]any{"data": route})
}

func (h *Handler) deleteRoute(writer http.ResponseWriter, request *http.Request) {
	revision, err := strconv.ParseInt(request.URL.Query().Get("revision"), 10, 64)
	if err != nil || revision < 1 {
		writeProblem(writer, http.StatusBadRequest, "invalid_revision", "revision query parameter must be a positive integer")
		return
	}
	if err := h.service.DeleteRoute(request.Context(), request.PathValue("id"), revision); err != nil {
		writeError(writer, err)
		return
	}
	writer.WriteHeader(http.StatusNoContent)
}

func (h *Handler) status(writer http.ResponseWriter, request *http.Request) {
	settings, err := h.service.Settings(request.Context())
	if err != nil {
		writeError(writer, err)
		return
	}
	writeJSON(writer, http.StatusOK, map[string]any{"data": map[string]any{
		"controller": "healthy", "zone": settings.Zone,
	}})
}

func decodeJSON(writer http.ResponseWriter, request *http.Request, target any) error {
	if !strings.HasPrefix(request.Header.Get("Content-Type"), "application/json") {
		return errors.New("Content-Type must be application/json")
	}
	decoder := json.NewDecoder(http.MaxBytesReader(writer, request.Body, 1<<20))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(target); err != nil {
		return fmt.Errorf("invalid JSON body: %w", err)
	}
	return nil
}

func writeError(writer http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, store.ErrNotFound):
		writeProblem(writer, http.StatusNotFound, "not_found", "route not found")
	case errors.Is(err, store.ErrHostnameConflict):
		writeProblem(writer, http.StatusConflict, "hostname_conflict", "hostname is already configured")
	case errors.Is(err, store.ErrRevisionConflict):
		writeProblem(writer, http.StatusConflict, "revision_conflict", "route changed; reload and try again")
	default:
		status := http.StatusBadRequest
		code := "validation_error"
		if strings.Contains(err.Error(), "caddy") {
			status, code = http.StatusBadGateway, "proxy_configuration_rejected"
		}
		writeProblem(writer, status, code, err.Error())
	}
}

func writeProblem(writer http.ResponseWriter, status int, code, detail string) {
	writer.Header().Set("Content-Type", "application/problem+json")
	writer.WriteHeader(status)
	_ = json.NewEncoder(writer).Encode(problem{Type: "https://lrp.local/problems/" + code, Title: strings.ReplaceAll(code, "_", " "), Status: status, Detail: detail})
}

func writeJSON(writer http.ResponseWriter, status int, value any) {
	writer.Header().Set("Content-Type", "application/json")
	writer.WriteHeader(status)
	_ = json.NewEncoder(writer).Encode(value)
}

func loopbackOnly(next http.Handler) http.Handler {
	return http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		host, _, err := net.SplitHostPort(request.Host)
		if err != nil {
			host = request.Host
		}
		if host != "localhost" && net.ParseIP(strings.Trim(host, "[]")) == nil {
			writeProblem(writer, http.StatusForbidden, "invalid_host", "dashboard accepts only loopback hostnames")
			return
		}
		ip := net.ParseIP(strings.Trim(host, "[]"))
		if ip != nil && !ip.IsLoopback() {
			writeProblem(writer, http.StatusForbidden, "invalid_host", "dashboard accepts only loopback addresses")
			return
		}
		next.ServeHTTP(writer, request)
	})
}

func securityHeaders(next http.Handler) http.Handler {
	return http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		writer.Header().Set("X-Content-Type-Options", "nosniff")
		writer.Header().Set("X-Frame-Options", "DENY")
		writer.Header().Set("Referrer-Policy", "no-referrer")
		writer.Header().Set("Content-Security-Policy", "default-src 'self'; style-src 'self'; script-src 'self'; frame-ancestors 'none'; base-uri 'none'; form-action 'self'")
		next.ServeHTTP(writer, request)
	})
}

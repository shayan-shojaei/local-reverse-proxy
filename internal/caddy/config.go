package caddy

import (
	"fmt"
	"net"

	"github.com/shayan/local-reverse-proxy/internal/domain"
)

func Render(zone string, routes []domain.Route) (map[string]any, error) {
	if err := domain.ValidateZone(zone); err != nil {
		return nil, err
	}
	httpRoutes := make([]any, 0, len(routes))
	httpsRoutes := make([]any, 0, len(routes))
	tlsSubjects := make([]any, 0, len(routes))
	for _, route := range routes {
		if !route.Enabled {
			continue
		}
		host := route.Hostname + "." + zone
		proxy := reverseProxy(route.Upstream)
		if route.PublicMode == domain.PublicHTTPS {
			httpRoutes = append(httpRoutes, routeForHost(host, map[string]any{
				"handler": "static_response", "status_code": 308,
				"headers": map[string]any{"Location": []string{"https://{http.request.host}{http.request.uri}"}},
			}))
			httpsRoutes = append(httpsRoutes, routeForHost(host, proxy))
			tlsSubjects = append(tlsSubjects, host)
		} else {
			httpRoutes = append(httpRoutes, routeForHost(host, proxy))
		}
	}
	servers := map[string]any{
		"http": map[string]any{"listen": []string{"0.0.0.0:80"}, "routes": httpRoutes},
	}
	apps := map[string]any{
		"http": map[string]any{"servers": servers},
		"pki":  map[string]any{"certificate_authorities": map[string]any{"local": map[string]any{"name": "LRP Local Authority"}}},
	}
	if len(httpsRoutes) > 0 {
		servers["https"] = map[string]any{
			"listen": []string{"0.0.0.0:443"}, "routes": httpsRoutes,
			"tls_connection_policies": []any{map[string]any{}},
		}
		apps["tls"] = map[string]any{"automation": map[string]any{"policies": []any{map[string]any{
			"subjects": tlsSubjects, "issuers": []any{map[string]any{"module": "internal"}},
		}}}}
	}
	return map[string]any{"admin": map[string]any{"listen": "0.0.0.0:2019"}, "apps": apps}, nil
}

func routeForHost(host string, handler map[string]any) map[string]any {
	return map[string]any{
		"match":    []any{map[string]any{"host": []string{host}}},
		"handle":   []any{handler},
		"terminal": true,
	}
}

func reverseProxy(upstream domain.Upstream) map[string]any {
	dialHost := upstream.Host
	if dialHost == "localhost" || dialHost == "127.0.0.1" || dialHost == "::1" || dialHost == "[::1]" {
		dialHost = "host.docker.internal"
	}
	if net.ParseIP(dialHost) != nil {
		dialHost = net.JoinHostPort(dialHost, fmt.Sprint(upstream.Port))
	} else {
		dialHost = fmt.Sprintf("%s:%d", dialHost, upstream.Port)
	}
	handler := map[string]any{
		"handler":   "reverse_proxy",
		"upstreams": []any{map[string]any{"dial": dialHost}},
	}
	if upstream.Scheme == "https" {
		tls := map[string]any{}
		if upstream.SkipTLSVerify {
			tls["insecure_skip_verify"] = true
		}
		handler["transport"] = map[string]any{"protocol": "http", "tls": tls}
	}
	return handler
}

# Architecture

## Trust boundary

`lrp` is the only host-native component. It writes an app-owned install directory, invokes Docker Compose, and requests `sudo` only for split-DNS and trust-store operations. The controller cannot execute host commands and does not mount the Docker socket.

The controller API accepts only loopback Host headers. Generated installations require either the bootstrap bearer token or an expiring HttpOnly browser session. Cookie-authenticated mutations require a loopback Origin header.

## Route transaction

For each mutation, the controller validates the request, loads the current route set, renders a complete Caddy JSON configuration, and submits it through Caddy's atomic `/load` endpoint. The SQLite mutation occurs only after Caddy accepts the snapshot. If persistence then fails, the controller reapplies the previous snapshot.

SQLite enforces unique relative hostnames and optimistic route revisions. Startup reconciles persisted desired state back into Caddy, so a controller or proxy restart converges without manual action.

## Networking

CoreDNS answers every name beneath the configured `.test` zone with `127.0.0.1`. Host split-DNS sends only that zone to CoreDNS. Caddy publishes ports 80 and 443 on loopback and matches exact Host headers. Unknown hosts receive no configured handler.

Targets named `localhost`, `127.0.0.1`, or `::1` are rewritten to `host.docker.internal`, backed by Docker's host-gateway mapping on Linux. Other hostnames and IPs are resolved from the Caddy container.

## Persistent state

- `controller-data`: SQLite desired state
- `caddy-data`: root CA, intermediates, and leaf certificates
- `caddy-config`: Caddy runtime state
- host config directory: Compose bundle, selected zone/version, bootstrap token, and exported CA certificate

Uninstall retains Docker volumes unless `--purge` is supplied.

# Local Reverse Proxy

Local Reverse Proxy (`lrp`) gives development servers memorable `.test` domains and locally trusted HTTPS. It runs a loopback-only dashboard plus stock Caddy and CoreDNS in Docker; a small host CLI owns the privileged DNS and certificate-trust setup.

## What works

- Exact local hostnames beneath a user-selected `.test` zone
- HTTP-only routes or HTTPS routes with automatic HTTP redirects
- HTTP and HTTPS upstreams, including an explicit self-signed-certificate escape hatch
- IPv4, IPv6, DNS-name, and host-local targets
- Atomic Caddy configuration updates with rollback on persistence failure
- Route CRUD from an accessible, responsive dashboard
- Bootstrap-token login and loopback Host/Origin protection
- macOS split DNS and system keychain trust
- systemd-resolved Linux split DNS and Debian-family system trust
- Version-pinned Compose installs, diagnostics, upgrades, and removable persistent data

## Requirements

- macOS with Docker Desktop, or a systemd-based Linux desktop with rootful Docker Engine
- Free loopback ports `53`, `80`, and `443`; dashboard port `7400` is configurable
- `sudo` for the one-time split-DNS and certificate-trust changes

Windows, LAN exposure, Podman, rootless Docker, wildcard routes, path routing, and container discovery are intentionally outside v1.

## Install

Install the correct release for your Mac or Linux machine, verify its checksum, configure Docker, split DNS, and local CA trust with one command:

```sh
curl -fsSL https://raw.githubusercontent.com/shayan/local-reverse-proxy/main/install.sh | sh
```

Choose another reserved `.test` zone or dashboard port through environment variables:

```sh
curl -fsSL https://raw.githubusercontent.com/shayan/local-reverse-proxy/main/install.sh | LRP_ZONE=dev.test LRP_DASHBOARD_PORT=7401 sh
```

The script can be downloaded and inspected before running. Set `LRP_SKIP_SETUP=1` to install only the CLI.

### Manual release install

Download the archive and matching `.sha256` file for your platform from GitHub Releases, verify them, then place `lrp` on your `PATH`:

```sh
lrp install --zone local.test
lrp dashboard
```

Preview every host-level operation without changing the machine:

```sh
lrp install --zone dev.test --dry-run
```

Useful lifecycle commands:

```sh
lrp doctor
lrp upgrade --version v0.2.0
lrp uninstall          # keeps Docker volumes
lrp uninstall --purge  # also deletes route and CA data
```

`lrp dashboard` passes the private bootstrap token in a URL fragment, exchanges it for an HttpOnly local session, and immediately removes the fragment from browser history.

## Develop locally

```sh
make test
docker compose up --build
```

For the UI development server:

```sh
cd web
npm ci
npm run dev
```

Manual Compose startup leaves authentication disabled unless `LRP_ADMIN_TOKEN` is set. The `lrp install` workflow always generates one.

## Architecture

- `lrp-server`: Go REST API, SQLite desired state, Caddy reconciliation, and static dashboard hosting
- Caddy: reverse proxy, HTTPS redirect, internal PKI, and leaf certificate lifecycle
- CoreDNS: authoritative wildcard answer for the selected `.test` zone
- `lrp`: host bootstrap and lifecycle CLI; the only component allowed to invoke privileged host commands

Caddy's Admin API is available only on the Compose network. No service mounts the Docker socket. Public traffic and the dashboard bind only to `127.0.0.1`.

The documented API contract lives in [`api/openapi.yaml`](api/openapi.yaml). Architectural and security details live in [`docs/architecture.md`](docs/architecture.md).

## Troubleshooting

Run `lrp doctor` first. Common failures are:

- Port conflict: stop the existing service on `53`, `80`, or `443`; `lrp` never kills it automatically.
- Browser still warns after install: restart the browser so it reloads the system trust store.
- Linux DNS fails after reboot: rerun `lrp install` on distributions that do not persist `resolvectl` loopback state.
- A target at `localhost`: `lrp` automatically maps loopback upstreams to Docker's `host.docker.internal` gateway.

## License

Apache-2.0. See [`LICENSE`](LICENSE).

#!/bin/sh
set -eu

REPOSITORY=${LRP_REPOSITORY:-shayan/local-reverse-proxy}
RELEASE=${LRP_VERSION:-latest}
ZONE=${LRP_ZONE:-local.test}
DASHBOARD_PORT=${LRP_DASHBOARD_PORT:-7400}
BIN_DIR=${LRP_BIN_DIR:-/usr/local/bin}

fail() {
  printf 'lrp installer: %s\n' "$1" >&2
  exit 1
}

command -v curl >/dev/null 2>&1 || fail "curl is required"
command -v tar >/dev/null 2>&1 || fail "tar is required"
command -v docker >/dev/null 2>&1 || fail "Docker must be installed before Local Reverse Proxy"

case "$(uname -s)" in
  Darwin) platform=darwin ;;
  Linux) platform=linux ;;
  *) fail "only macOS and Linux are supported" ;;
esac

case "$(uname -m)" in
  x86_64|amd64) architecture=amd64 ;;
  arm64|aarch64) architecture=arm64 ;;
  *) fail "unsupported CPU architecture: $(uname -m)" ;;
esac

archive="lrp-${platform}-${architecture}.tar.gz"
if [ "$RELEASE" = latest ]; then
  base_url="https://github.com/${REPOSITORY}/releases/latest/download"
  image_version=latest
else
  case "$RELEASE" in v*) ;; *) RELEASE="v${RELEASE}" ;; esac
  base_url="https://github.com/${REPOSITORY}/releases/download/${RELEASE}"
  image_version=${RELEASE#v}
fi

temporary_directory=$(mktemp -d "${TMPDIR:-/tmp}/lrp-install.XXXXXX")
trap 'rm -rf "$temporary_directory"' EXIT HUP INT TERM

printf 'Downloading Local Reverse Proxy for %s/%s…\n' "$platform" "$architecture"
curl -fL --retry 3 --proto '=https' --tlsv1.2 -o "$temporary_directory/$archive" "$base_url/$archive"
curl -fL --retry 3 --proto '=https' --tlsv1.2 -o "$temporary_directory/$archive.sha256" "$base_url/$archive.sha256"

(
  cd "$temporary_directory"
  if command -v sha256sum >/dev/null 2>&1; then
    sha256sum -c "$archive.sha256"
  else
    shasum -a 256 -c "$archive.sha256"
  fi
  tar -xzf "$archive"
)

if [ -d "$BIN_DIR" ] && [ -w "$BIN_DIR" ]; then
  install -m 0755 "$temporary_directory/lrp" "$BIN_DIR/lrp"
  lrp_command=$BIN_DIR/lrp
elif command -v sudo >/dev/null 2>&1; then
  sudo install -m 0755 "$temporary_directory/lrp" "$BIN_DIR/lrp"
  lrp_command=$BIN_DIR/lrp
else
  destination=${HOME}/.local/bin
  mkdir -p "$destination"
  install -m 0755 "$temporary_directory/lrp" "$destination/lrp"
  lrp_command=$destination/lrp
  printf 'Installed lrp in %s; add that directory to PATH.\n' "$destination"
fi

printf 'Installed the lrp CLI. Starting host setup…\n'
if [ "${LRP_SKIP_SETUP:-0}" = 1 ]; then
  printf 'Setup skipped. Run: lrp install --zone %s\n' "$ZONE"
  exit 0
fi

"$lrp_command" install --zone "$ZONE" --dashboard-port "$DASHBOARD_PORT" --version "$image_version"
printf '\nInstallation complete. Open the dashboard with: lrp dashboard\n'

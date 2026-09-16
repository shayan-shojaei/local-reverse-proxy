#!/bin/sh
set -eu

root=$(CDPATH= cd -- "$(dirname "$0")/.." && pwd)
fixture=$(mktemp -d "${TMPDIR:-/tmp}/lrp-install-test.XXXXXX")
trap 'rm -rf "$fixture"' EXIT HUP INT TERM
mkdir -p "$fixture/bin" "$fixture/output" "$fixture/release"

cat > "$fixture/release/lrp" <<'SCRIPT'
#!/bin/sh
printf '%s\n' "$*" > "${LRP_TEST_OUTPUT}/lrp-arguments"
SCRIPT
chmod +x "$fixture/release/lrp"
tar -C "$fixture/release" -czf "$fixture/release/lrp-darwin-arm64.tar.gz" lrp
(
  cd "$fixture/release"
  shasum -a 256 lrp-darwin-arm64.tar.gz > lrp-darwin-arm64.tar.gz.sha256
)

cat > "$fixture/bin/uname" <<'SCRIPT'
#!/bin/sh
if [ "${1:-}" = -s ]; then printf 'Darwin\n'; else printf 'arm64\n'; fi
SCRIPT
cat > "$fixture/bin/docker" <<'SCRIPT'
#!/bin/sh
exit 0
SCRIPT
cat > "$fixture/bin/curl" <<'SCRIPT'
#!/bin/sh
destination=
while [ "$#" -gt 0 ]; do
  if [ "$1" = -o ]; then destination=$2; shift 2; continue; fi
  url=$1
  shift
done
cp "${LRP_TEST_FIXTURE}/$(basename "$url")" "$destination"
SCRIPT
chmod +x "$fixture/bin/uname" "$fixture/bin/docker" "$fixture/bin/curl"

PATH="$fixture/bin:$PATH" \
LRP_TEST_FIXTURE="$fixture/release" \
LRP_TEST_OUTPUT="$fixture/output" \
LRP_BIN_DIR="$fixture/output" \
LRP_ZONE=dev.test \
LRP_DASHBOARD_PORT=7444 \
sh "$root/install.sh" >/dev/null

grep -q '^install --zone dev.test --dashboard-port 7444 --version latest$' "$fixture/output/lrp-arguments"
test -x "$fixture/output/lrp"
printf 'install.sh integration test passed\n'

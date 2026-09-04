#!/usr/bin/env bash
# Ship saveinator binaries: cross-compile locally for linux/amd64, scp them to
# the VPS, and recreate the containers (docker-compose.override.yml on the VPS
# bind-mounts ./binaries/<svc> over the image copies).
#
# NEVER docker-build the Go images on the VPS: it is a 1-CPU box and the build
# drives load average past 100, which stalls sshd (banner exchange timeouts).
#
# Usage:
#   scripts/ship-binaries.sh                     # ship saveinator botd dash
#   scripts/ship-binaries.sh saveinator          # ship one service
#   scripts/ship-binaries.sh botd dash --migrate # ship + run DB migrations
#
# Env overrides: VPS_HOST (default 45.128.235.219), VPS_USER (root),
# APP_DIR (/opt/saveinator).
set -euo pipefail

VPS_HOST="${VPS_HOST:-45.128.235.219}"
VPS_USER="${VPS_USER:-root}"
APP_DIR="${APP_DIR:-/opt/saveinator}"

# Container path of each service binary (matches the Dockerfile COPY lines).
bin_path_for() {
  case "$1" in
    saveinator) echo "/app/saveinator" ;;
    botd) echo "/app/botd" ;;
    dash) echo "/app/dash" ;;
    *) return 1 ;;
  esac
}

md5_of() {
  if command -v md5 >/dev/null 2>&1; then
    md5 -q "$1"
  else
    md5sum "$1" | cut -d' ' -f1
  fi
}

migrate=0
services=()
for arg in "$@"; do
  case "$arg" in
    --migrate) migrate=1 ;;
    -h|--help) sed -n '2,14p' "$0"; exit 0 ;;
    *) services+=("$arg") ;;
  esac
done
if [ ${#services[@]} -eq 0 ]; then
  services=(saveinator botd dash)
fi
for svc in "${services[@]}"; do
  if ! bin_path_for "$svc" >/dev/null; then
    echo "unknown service: $svc (known: saveinator botd dash)" >&2
    exit 1
  fi
done

SSH="ssh -o BatchMode=yes -o ConnectTimeout=20 $VPS_USER@$VPS_HOST"
repo_root="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
out_dir="$(mktemp -d /tmp/saveinator-ship.XXXXXX)"
trap 'rm -rf "$out_dir"' EXIT

echo "=== [1/5] Building linux/amd64 binaries: ${services[*]} ==="
for svc in "${services[@]}"; do
  (cd "$repo_root/go" && CGO_ENABLED=0 GOOS=linux GOARCH=amd64 \
    go build -ldflags="-s -w" -o "$out_dir/$svc" "./cmd/$svc")
done

echo "=== [2/5] Shipping to $VPS_USER@$VPS_HOST:$APP_DIR/binaries ==="
# Stage under temp names and mv into place: the running container executes
# the current binary inode, and scp-over-it fails with ETXTBSY ("dest open
# failure"). mv swaps the directory entry atomically, so the old binary
# keeps running until the container is recreated.
$SSH "mkdir -p '$APP_DIR/binaries/.ship-tmp'"
for svc in "${services[@]}"; do
  scp -o BatchMode=yes -C "$out_dir/$svc" "$VPS_USER@$VPS_HOST:$APP_DIR/binaries/.ship-tmp/$svc"
done
mv_cmd=""
for svc in "${services[@]}"; do
  mv_cmd="$mv_cmd mv -f .ship-tmp/$svc $svc &&"
done
$SSH "cd '$APP_DIR/binaries' && ${mv_cmd%&&} && chmod 755 ${services[*]}"

echo "=== [3/5] Ensuring compose override (binary bind-mounts) ==="
$SSH "cd '$APP_DIR' && if [ ! -f docker-compose.override.yml ]; then
  cat > docker-compose.override.yml <<'OVERRIDE'
# Prebuilt-binary deploy: bind-mount locally cross-compiled linux/amd64
# binaries over the image copies. Managed by scripts/ship-binaries.sh.
services:
  saveinator:
    volumes:
      - ./binaries/saveinator:/app/saveinator:ro
  botd:
    volumes:
      - ./binaries/botd:/app/botd:ro
  dash:
    volumes:
      - ./binaries/dash:/app/dash:ro
OVERRIDE
  echo 'override created';
else echo 'override present'; fi"

echo "=== [4/5] Recreating containers: ${services[*]} ==="
$SSH "cd '$APP_DIR' && docker compose up -d --force-recreate ${services[*]}"

if [ "$migrate" -eq 1 ]; then
  echo "=== [4b] Running DB migrations (rebuilding stale migrate image) ==="
  # The migrate image bakes db/ in; without a rebuild it silently no-ops on
  # old migrations even though db/ changed locally.
  $SSH "cd '$APP_DIR' && docker compose --profile tools build migrate && docker compose --profile tools run --rm migrate"
fi

echo "=== [5/5] Verifying ==="
for svc in "${services[@]}"; do
  local_md5="$(md5_of "$out_dir/$svc")"
  remote_md5="$($SSH "md5sum '$APP_DIR/binaries/$svc' | cut -d' ' -f1")"
  if [ "$local_md5" != "$remote_md5" ]; then
    echo "MD5 MISMATCH for $svc (local $local_md5 vs remote $remote_md5)" >&2
    exit 1
  fi
  echo "$svc: binary md5 OK"
done
$SSH "cd '$APP_DIR' && docker compose ps --format '{{.Name}} {{.Status}}' | grep -E 'saveinator-saveinator|botd|dash'"
$SSH "cd '$APP_DIR' && docker compose logs --since 1m ${services[*]} 2>&1 | grep -iE 'error|fatal|panic' | head -5 || true"

echo "=== Shipped: ${services[*]} ==="

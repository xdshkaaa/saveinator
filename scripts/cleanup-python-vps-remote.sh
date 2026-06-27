#!/usr/bin/env bash
set -euo pipefail
APP_DIR="${APP_DIR:-/opt/saveinator}"
cd "$APP_DIR"

docker stop saveinator-bot-1 saveinator-worker-1 2>/dev/null || true
docker rm saveinator-bot-1 saveinator-worker-1 2>/dev/null || true
docker compose stop bot worker 2>/dev/null || true
docker compose rm -f bot worker 2>/dev/null || true

docker image prune -f
for pattern in saveinator-bot saveinator-worker; do
  ids=$(docker images --format '{{.Repository}}:{{.Tag}} {{.ID}}' | awk -v p="$pattern" '$1 ~ p {print $2}')
  if [ -n "$ids" ]; then
    docker rmi $ids 2>/dev/null || true
  fi
done

rm -rf /tmp/prometheus_multiproc 2>/dev/null || true
systemctl daemon-reload 2>/dev/null || true
systemctl restart ytbot 2>/dev/null || true
docker compose ps

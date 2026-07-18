#!/usr/bin/env bash
# Cut over production VPS from legacy Python/Celery to Go saveinator.
set -euo pipefail

VPS_HOST="${VPS_HOST:?set VPS_HOST env var}"
VPS_USER="${VPS_USER:-root}"
APP_DIR="${APP_DIR:-/opt/saveinator}"

echo "=== Cutover to Go on $VPS_HOST ==="

ssh "$VPS_USER@$VPS_HOST" bash -s <<EOF
set -euo pipefail
cd '$APP_DIR'

echo "[1/7] Backup PostgreSQL..."
docker compose exec -T db pg_dump -U saveinator saveinator > /tmp/saveinator-pre-go-\$(date +%Y%m%d%H%M).dump
ls -lh /tmp/saveinator-pre-go-*.dump | tail -1

echo "[2/7] Stop legacy Python services (if running)..."
docker stop saveinator-bot-1 saveinator-worker-1 2>/dev/null || true
docker rm saveinator-bot-1 saveinator-worker-1 2>/dev/null || true
docker compose stop bot worker 2>/dev/null || true
docker compose rm -f bot worker 2>/dev/null || true

echo "[3/7] Ensure webhook mode..."
if grep -q '^USE_POLLING=true' .env 2>/dev/null; then
  sed -i 's/^USE_POLLING=.*/USE_POLLING=false/' .env
fi

echo "[4/7] Build and start Go saveinator..."
docker compose build saveinator
docker compose up -d --force-recreate saveinator db redis

echo "[5/7] Run migrations..."
docker compose --profile tools build migrate
docker compose --profile tools run --rm migrate || true

echo "[6/7] Verify health and metrics..."
sleep 8
curl -fsS http://127.0.0.1:8000/health
curl -fsS http://127.0.0.1:9101/metrics | grep saveinator_uptime_seconds | head -1
curl -fsS http://127.0.0.1:9102/metrics | grep saveinator_worker_uptime_seconds | head -1

echo "[7/7] Service status..."
docker compose ps
docker compose logs --tail=30 saveinator
EOF

echo "=== Cutover complete. Check Grafana at https://saveinator.xdshka.party ==="

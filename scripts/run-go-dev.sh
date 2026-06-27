#!/usr/bin/env bash
# Start Go saveinator locally with polling (separate bot token, does not touch production).
set -euo pipefail

ROOT="$(cd "$(dirname "$0")/.." && pwd)"
ENV_FILE="${ROOT}/.env.go.dev"

if [[ ! -f "$ENV_FILE" ]]; then
  echo "Missing $ENV_FILE — copy from .env.example and set BOT_TOKEN"
  exit 1
fi

echo "=== Starting dev Postgres + Redis ==="
docker compose -f "$ROOT/docker-compose.dev.yml" up -d

echo "=== Waiting for Postgres ==="
for i in $(seq 1 30); do
  if docker compose -f "$ROOT/docker-compose.dev.yml" exec -T db pg_isready -U saveinator >/dev/null 2>&1; then
    break
  fi
  sleep 1
done

echo "=== Bootstrapping database schema ==="
set -a
# shellcheck disable=SC1090
source "$ENV_FILE"
set +a
export DB_PASSWORD="${DB_PASSWORD:-saveinator}"
cd "$ROOT"
docker compose -f "$ROOT/docker-compose.dev.yml" --profile tools build migrate
docker compose -f "$ROOT/docker-compose.dev.yml" --profile tools run --rm migrate || true

echo "=== Building Go binary ==="
cd "$ROOT/go"
go build -o "$ROOT/bin/saveinator-go-dev" ./cmd/saveinator

echo "=== Starting Go bot (polling) ==="
set -a
source "$ENV_FILE"
set +a
exec "$ROOT/bin/saveinator-go-dev"

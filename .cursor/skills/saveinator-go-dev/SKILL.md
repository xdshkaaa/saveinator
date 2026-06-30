---
name: saveinator-go-dev
description: >-
  Runs the Saveinator Go bot locally (polling, dev Postgres/Redis, schema bootstrap).
  Use when starting local Go development, run-go-dev, polling mode, SAVEINATOR_MODE,
  or debugging the Go binary without touching production.
---

# Saveinator Go Dev Loop

Saveinator Go replaces Python `bot` + Celery `worker` with one binary (`go/cmd/saveinator`).
Schema migrations live in Python Alembic only — Go uses the existing PostgreSQL schema.

## Prerequisites

- `.env.go.dev` at repo root (copy from `.env.example`, **separate dev BOT_TOKEN**)
- Docker (Postgres + Redis via `docker-compose.dev.yml`)
- `uv` + Python deps (schema bootstrap in dev script)
- Local `ffmpeg` + `yt-dlp` if running outside Docker

## Quick start (recommended)

```bash
scripts/run-go-dev.sh
```

This script:
1. Starts `docker-compose.dev.yml` (Postgres + Redis)
2. Bootstraps schema via Python `Base.metadata.create_all` + `alembic stamp head`
3. Builds `bin/saveinator-go-dev`
4. Runs the binary with `.env.go.dev`

## Manual run

```bash
docker compose -f docker-compose.dev.yml up -d

cd go
export BOT_TOKEN=...          # from .env.go.dev
export DATABASE_URL=postgres://saveinator:saveinator@localhost:5432/saveinator
export REDIS_URL=redis://localhost:6379/0
export USE_POLLING=true
go run ./cmd/saveinator
```

## Env loading order (Go)

`go/internal/config/config.go` loads: `.env.go.dev`, `.env`, `../.env.go.dev`, `../.env`.

| Variable | Notes |
|----------|-------|
| `SAVEINATOR_MODE` | `all` (default), `bot`, `worker` |
| `DATABASE_URL` | Go normalizes `postgresql+asyncpg://` → `postgres://` |
| `USE_POLLING` | `true` for local dev without webhook |
| `LOG_LEVEL` | `DEBUG` for verbose slog JSON logs |

## Verify running

```bash
curl -s localhost:9101/metrics | head
# expect saveinator_* counters
```

Redis user lock pattern: `user_busy:{user_id}` (value `{scene}:{token}`).

## Gotchas

- **Never use prod BOT_TOKEN** in `.env.go.dev`
- Locale changes require **rebuild** (`//go:embed` in `go/internal/locale/`)
- New DB tables → Alembic migration in `db/migrations/`, not Go code
- Prod path: `docker compose -f docker-compose.go.yml` (not `scripts/deploy.sh` Python stack)
- `scripts/deploy.sh` still deploys legacy Python `bot`+`worker` — confirm stack before deploy

## Key paths

| Area | Path |
|------|------|
| Entry | `go/cmd/saveinator/main.go` |
| App wiring | `go/internal/app/app.go` |
| Config | `go/internal/config/config.go` |
| Handlers | `go/internal/handler/` |
| Workers | `go/internal/worker/` |
| Queue | `go/internal/queue/` |
| Docs | `go/README.md` |

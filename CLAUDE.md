# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## What this is

Telegram bots (Go binaries) that download media from YouTube, TikTok, X/Twitter, Spotify, SoundCloud, and Pinterest. Two services: `saveinator` (main) and `pinterest` (microservice). Schema managed via Alembic in Python; runtime is pure Go.

## Commands

### Local dev

```bash
# Start Postgres + Redis, bootstrap schema, build + run (all-in-one)
scripts/run-go-dev.sh

# Manual
docker compose -f docker-compose.dev.yml up -d
cd go && USE_POLLING=true go run ./cmd/saveinator
cd go && go run ./services/pinterest/cmd
```

Requires `.env.go.dev` at repo root (separate dev BOT_TOKEN — never use prod token).

### Build

```bash
cd go && go build -o ../bin/saveinator ./cmd/saveinator
cd go && go build -o ../bin/pinterest ./services/pinterest/cmd
```

### Test

```bash
cd go && go build ./... && go test -race -count=1 -coverprofile=coverage.out ./...

# Targeted
cd go && go test -v ./internal/linkparser/...
cd go && go test -run TestName ./internal/...
```

### Locale parity check

```bash
scripts/check-parity.sh    # diff keys across all 4 JSON files
scripts/sync-locales.sh    # copy root → go/internal/locale/locales/
```

### DB migrations

```bash
# Local
docker compose -f docker-compose.dev.yml --profile tools run --rm migrate

# Production
docker compose --profile tools run --rm migrate
```

### Deploy

```bash
./scripts/deploy.sh          # full VPS deploy (sync + build + migrate)
./scripts/hotfix-worker.sh   # code sync + rebuild only
```

## Architecture

```
Telegram → Caddy :8093 → saveinator :8000 (webhook)
                       → pinterest  :8001 (webhook)
pinterest → POST /download/pinterest (HTTP API)
saveinator / pinterest → PostgreSQL (shared schema)
saveinator / pinterest → Redis (asynq queue, locks, rate limits)
saveinator → yt-dlp / ffmpeg (in-process worker)
Prometheus → :9101 + :9102 + :9103 → Grafana
```

**Mode control** (`SAVEINATOR_MODE`): `all` (default) runs webhook + asynq worker + metrics in one process; `bot` or `worker` for split deployments.

## Code layout

```
go/cmd/saveinator/       — main binary entrypoint
go/services/pinterest/   — Pinterest microservice (own bot, worker, HTTP API)
go/services/pinterest_kz/ — Pinterest KZ variant
go/services/spotify/     — Spotify microservice
go/internal/
  app/        — wires bot + worker + metrics, handles SAVEINATOR_MODE
  config/     — env loading (reads .env.go.dev, .env, ../.env.go.dev, ../.env)
  handler/    — Telegram update routing + callback dispatch
  worker/     — asynq task handlers (download, broadcast, maintenance)
  queue/      — task type constants + EnqueueX() functions + payload structs
  runtime/    — Redis-backed admin-tunable settings (hot-swap without restart)
  db/         — pgx/v5 store; testdata/schema.sql for integration tests
  linkparser/ — URL detection and platform identification
  locale/     — //go:embed JSON files; rebuild required after edits
  redisx/     — Redis client + user lock (user_busy:{userID})
  ytdlp/      — yt-dlp subprocess wrapper
  pinterest/  — Pinterest API client (pins, boards, short links)
  spotify/    — Spotify Web API
  soundcloud/ — SoundCloud via yt-dlp metadata
  tiktok/     — TikTok via yt-dlp
  youtube/    — YouTube quality/ratio session + yt-dlp
  sender/     — Telegram send helpers (video, audio, photo groups)
locales/              — canonical EN/RU JSON (copy to go/internal/locale/locales/)
db/migrations/        — Alembic revision history
monitoring/           — Prometheus, Grafana, Loki, Alertmanager configs
```

## Key patterns

### Download flow

1. `handler/bot.go` `dispatchLink()` — detects platform via `linkparser`, checks `runtime.PlatformEnabled`
2. Handler acquires user lock (`redisx`), enqueues task via `queue.EnqueueX()`
3. Worker `handleX(ctx, *asynq.Task)` — downloads + sends via `sender`
4. Cancel path: `dlc:` / `dlq:` callbacks release lock via `handler/cancel.go`

All download tasks use `MaxRetry(0)` — no automatic retry.

### Adding a new platform

Follow order: `linkparser/parser.go` → `handler/bot.go dispatchLink()` → `handler/<platform>.go` → `queue/client.go` (TypeX constant + EnqueueX + payload) → `worker/<platform>.go` → register in `worker/deps.go` → locales (4 files) → `runtime/registry.go` if admin-tunable → `.env.example` + `config/config.go`.

### Locales (4 files must stay in sync)

- `locales/en.json` + `locales/ru.json` — canonical
- `go/internal/locale/locales/en.json` + `ru.json` — Go embed (rebuild required)

Use `{var}` placeholders. Run `scripts/check-parity.sh` after any locale change.

### Schema changes

Edit `db/models.py` → write Alembic revision in `db/migrations/versions/` → run migrate container → update `go/internal/db/testdata/schema.sql` if integration tests cover new tables.

Go never creates tables. `DATABASE_URL` with `postgresql+asyncpg://` scheme is normalized to `postgres://` in `config.go`.

### Runtime settings

`go/internal/runtime/registry.go` defines admin-tunable keys (int/bool/enum/list). Values stored in Redis, hot-swapped without restart. Labels exposed in `/admin` panel via `LabelEN`/`LabelRU`.

### Callback data

Max 64 bytes (Telegram limit). Registered prefixes: `lang|`, `quality:`, `ratio:`, `settings|`, `dlc:`, `dlq:`, `admin|`, `broadcast|`, `ttk:img:`.

## CI jobs

| Job | Command |
|-----|---------|
| `go` | `go build ./... && go test -race -count=1 ./...` (Go 1.22.3) |
| `migrate-image` | `docker build -f docker/migrate/Dockerfile .` |
| `compose` | `docker compose config -q` |

## Env files

| File | Purpose |
|------|---------|
| `.env` | Production |
| `.env.go.dev` | Local dev (separate BOT_TOKEN) |
| `.env.example` | Template |
| `.env.monitoring.example` | Monitoring stack template |

Key vars: `BOT_TOKEN`, `PINTEREST_BOT_TOKEN`, `DATABASE_URL`, `REDIS_URL`, `SAVEINATOR_MODE`, `USE_POLLING`, `METRICS_PORT`/`WORKER_METRICS_PORT`.

## Production VPS

Host: `103.214.69.38`, app dir: `/opt/saveinator`, branch: `main`.

Verify after deploy:
```bash
ssh root@103.214.69.38 'curl -fsS http://127.0.0.1:8000/health'
ssh root@103.214.69.38 'curl -fsS http://127.0.0.1:9101/metrics | head'
```

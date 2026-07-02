# Saveinator

Telegram bots for downloading media from YouTube, TikTok, X/Twitter, Spotify, and SoundCloud — plus a dedicated Pinterest bot.

Production runs as **Go binaries** (webhook/polling + asynq worker + Prometheus metrics). See [`go/README.md`](go/README.md) for development details.

## Features

- Video/photo downloads via yt-dlp (YouTube, TikTok, X) — main `saveinator` bot
- Pinterest pins and boards — separate `pinterest` microservice bot
- Spotify metadata (Web API) and optional audio via yt-dlp YouTube search
- SoundCloud metadata and optional audio via yt-dlp
- `POST /download/pinterest` HTTP API on the Pinterest service
- EN/RU localization, admin panel, broadcasts, `/clear` queue command
- Grafana dashboards at [saveinator.xdshka.party](https://saveinator.xdshka.party)

## Quick start (production Docker)

```bash
cp .env.example .env
# Edit BOT_TOKEN, PINTEREST_BOT_TOKEN, DB_PASSWORD, Spotify creds, etc.

docker compose up -d --build
docker compose --profile tools run --rm migrate   # first deploy / schema updates
```

Services:

| Service | Description |
|---------|-------------|
| `saveinator` | Main Go app: Telegram webhook `:8000`, asynq worker, metrics `:9101` + `:9102` |
| `pinterest` | Pinterest-only bot: webhook `:8001`, worker, metrics `:9103`, download API |
| `db` | PostgreSQL 16 (existing `pgdata` volume preserved on upgrade) |
| `redis` | Queue, locks, rate limits (shared) |

## Local development

```bash
docker compose -f docker-compose.dev.yml up -d   # Postgres + Redis
cd go && go run ./cmd/saveinator                 # main bot
cd go && go run ./services/pinterest/cmd         # pinterest bot (separate BOT_TOKEN)
```

Set `USE_POLLING=true` in `.env.go.dev` for local polling without webhook.

## Production webhook

Main bot:

```env
USE_POLLING=false
WEBHOOK_HOST=https://saveinator-hooks.xdshka.party
WEBHOOK_PATH=/webhook
WEBHOOK_SECRET_TOKEN=long-random-value
```

Pinterest bot (same host, different path/port):

```env
PINTEREST_WEBHOOK_PATH=/webhook/pinterest
PINTEREST_WEBHOOK_PORT=8001
PINTEREST_BOT_TOKEN=...
```

Public routes (via Caddy/Cloudflare): `/webhook`, `/webhook/pinterest`, `/download/pinterest`, `/health`. Metrics stay on `127.0.0.1:9101`, `:9102`, `:9103`.

## Deploy to VPS

```bash
./scripts/deploy.sh          # sync + build + up saveinator + pinterest
```

Legacy one-shot migration scripts (post Python cutover): `scripts/cutover-to-go.sh`, `scripts/cleanup-python-vps.sh`.

Monitoring: see [`MONITORING.md`](MONITORING.md).

## Tests

```bash
cd go && go test ./...
```

## Database migrations

Schema is managed with Alembic (`db/migrations/`). Run via migrate container:

```bash
docker compose --profile tools run --rm migrate
```

## Environment variables

See [`.env.example`](.env.example). Key values:

| Variable | Description |
|----------|-------------|
| `BOT_TOKEN` | Main Telegram bot token |
| `PINTEREST_BOT_TOKEN` | Pinterest Telegram bot token |
| `DATABASE_URL` | PostgreSQL URL (`postgresql+asyncpg://...` in Docker) |
| `DB_PASSWORD` | Postgres password for Compose |
| `REDIS_URL` | Redis for asynq and rate limits |
| `SAVEINATOR_MODE` | `all` (default), `bot`, or `worker` |
| `METRICS_PORT` / `WORKER_METRICS_PORT` | Main bot Prometheus ports (default `9101` / `9102`) |

## Architecture

```text
Telegram → Caddy :8093 → saveinator :8000 (webhook)
Telegram → Caddy :8093 → pinterest :8001 (webhook/pinterest)
pinterest → POST /download/pinterest (HTTP API)
saveinator / pinterest → PostgreSQL (users, downloads, broadcasts)
saveinator / pinterest → Redis (asynq queue, locks, runtime settings)
saveinator → yt-dlp / ffmpeg (downloads in-process worker)
Prometheus → :9101 + :9102 + :9103 → Grafana (saveinator.xdshka.party)
```

Code layout:

- [`go/`](go/) — application source
- [`go/services/pinterest/`](go/services/pinterest/) — Pinterest microservice
- [`locales/`](locales/) — EN/RU strings
- [`db/migrations/`](db/migrations/) — Alembic schema history
- [`monitoring/`](monitoring/) — Prometheus, Grafana, alerts

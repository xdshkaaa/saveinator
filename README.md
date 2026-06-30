# Saveinator

Telegram bot for downloading media from YouTube, TikTok, X/Twitter, Pinterest, Spotify, and SoundCloud.

Production runs as a **single Go binary** (webhook/polling + asynq worker + Prometheus metrics). See [`go/README.md`](go/README.md) for development details.

## Features

- Video/photo downloads via yt-dlp (YouTube, TikTok, X)
- Pinterest pins and boards
- Spotify metadata (Web API) and optional audio via yt-dlp YouTube search
- SoundCloud metadata and optional audio via yt-dlp
- `POST /download/pinterest` HTTP API
- EN/RU localization, admin panel, broadcasts, `/clear` queue command
- Grafana dashboards at [saveinator.xdshka.party](https://saveinator.xdshka.party)

## Quick start (production Docker)

```bash
cp .env.example .env
# Edit BOT_TOKEN, DB_PASSWORD, Spotify creds, etc.

docker compose up -d --build
docker compose --profile tools run --rm migrate   # first deploy / schema updates
```

Services:

| Service | Description |
|---------|-------------|
| `saveinator` | Go app: Telegram webhook, asynq worker, metrics `:9101` + `:9102` |
| `db` | PostgreSQL 16 (existing `pgdata` volume preserved on upgrade) |
| `redis` | Queue, locks, rate limits |

## Local development

```bash
docker compose -f docker-compose.dev.yml up -d   # Postgres + Redis
cd go && go run ./cmd/saveinator                 # or scripts/run-go-dev.sh
```

Set `USE_POLLING=true` in `.env.go.dev` for local polling without webhook.

## Production webhook

```env
USE_POLLING=false
WEBHOOK_HOST=https://saveinator-hooks.xdshka.party
WEBHOOK_PATH=/webhook
WEBHOOK_SECRET_TOKEN=long-random-value
DATABASE_URL=postgresql+asyncpg://saveinator:${DB_PASSWORD}@db:5432/saveinator
```

Public routes (via Caddy/Cloudflare): `/`, `/health`, `/webhook*`. Metrics stay on `127.0.0.1:9101` and `:9102`.

## Deploy to VPS

```bash
./scripts/deploy.sh          # sync + build + up saveinator
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
| `BOT_TOKEN` | Telegram bot token |
| `DATABASE_URL` | PostgreSQL URL (`postgresql+asyncpg://...` in Docker) |
| `DB_PASSWORD` | Postgres password for Compose |
| `REDIS_URL` | Redis for asynq and rate limits |
| `SAVEINATOR_MODE` | `all` (default), `bot`, or `worker` |
| `METRICS_PORT` / `WORKER_METRICS_PORT` | Prometheus scrape ports (default `9101` / `9102`) |

## Architecture

```text
Telegram → Caddy :8093 → saveinator :8000 (webhook)
saveinator → PostgreSQL (users, downloads, broadcasts)
saveinator → Redis (asynq queue, locks, runtime settings)
saveinator → yt-dlp / ffmpeg (downloads in-process worker)
Prometheus → :9101 + :9102 → Grafana (saveinator.xdshka.party)
```

Code layout:

- [`go/`](go/) — application source
- [`locales/`](locales/) — EN/RU strings
- [`db/migrations/`](db/migrations/) — Alembic schema history
- [`monitoring/`](monitoring/) — Prometheus, Grafana, alerts

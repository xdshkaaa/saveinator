# Saveinator Go rewrite

Lightweight Telegram downloader bot written in Go. Replaces the Python `bot` + Celery `worker` pair with a single binary.

## Modes

`SAVEINATOR_MODE` controls which components run in one process:

| Value | Runs |
|-------|------|
| `all` (default) | webhook/polling + asynq worker + metrics |
| `bot` | telegram + metrics only |
| `worker` | asynq worker only |

## Build

```bash
cd go
go build -o ../bin/saveinator ./cmd/saveinator
go build -o ../bin/pinterest ./services/pinterest/cmd
```

## Run locally

```bash
# Redis + Postgres
docker compose -f docker-compose.dev.yml up -d

cd go
export BOT_TOKEN=...
export DATABASE_URL=postgres://saveinator:password@localhost:5432/saveinator
export REDIS_URL=redis://localhost:6379/0
export USE_POLLING=true
go run ./cmd/saveinator

# Pinterest bot (separate token)
export BOT_TOKEN=$PINTEREST_BOT_TOKEN
go run ./services/pinterest/cmd
```

## Production (Docker)

Two containers: main `saveinator` and dedicated `pinterest` microservice:

```bash
docker compose up -d --build
```

| Container | Binary | Ports |
|-----------|--------|-------|
| `saveinator` | `cmd/saveinator` | `:8000`, `:9101`, `:9102` |
| `pinterest` | `services/pinterest/cmd` | `:8001`, `:9103` |

## Migrated features

- Webhook + polling
- `/start` onboarding (EN/RU), bot command menu
- `/settings` — language, default YouTube quality/ratio
- YouTube quality + aspect ratio picker with ffmpeg transcoding
- **Pinterest** — separate microservice (`go/services/pinterest/`): pins, boards, short links; HTTP API `POST /download/pinterest`
- **TikTok** — video + carousel slideshows via yt-dlp + image download
- **TikTok carousel button** — download photos from video posts (`ttk:img:` callback)
- **Spotify** — release cards via Spotify API; optional audio via YouTube search + yt-dlp
- **SoundCloud** — metadata via yt-dlp; optional audio download
- **Download cancel** — cancel button + active download queue (`dlc:` / `dlq:` callbacks)
- **Admin panel** — `/admin` runtime settings (Redis hot-swap), shadow bans, user stats
- **Broadcasts** — `/broadcast` create/send with asynq worker
- **Runtime settings** — global + platform int/bool/enum/list keys wired into workers and handlers
- **Maintenance** — hourly temp dir sweep, TikTok cookie refresh (5 min)
- **X/Twitter photo posts** — fxtwitter/vxtwitter fallback when yt-dlp finds no video
- **Group anti-spam** — banned links (DB) + duplicate URL dedup (Redis)
- Link parsing (YouTube, TikTok, X, Spotify, SoundCloud) in main bot; Pinterest in `services/pinterest`
- Video/image downloads via `yt-dlp` subprocess
- Rate limiting + per-user download lock (Redis)
- Prometheus metrics on `:9101` (`saveinator_downloads_enqueued_total`, rate limit counters)
- Shared PostgreSQL schema (Alembic migrations in `db/migrations/`)


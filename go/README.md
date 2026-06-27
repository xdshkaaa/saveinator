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
```

## Production (Docker)

Single container instead of separate `bot` and `worker` services:

```bash
docker compose -f docker-compose.go.yml up -d --build
```

## Migrated (phase 1–6)

- Webhook + polling
- `/start` onboarding (EN/RU)
- `/settings` — language, default YouTube quality/ratio
- YouTube quality + aspect ratio picker with ffmpeg transcoding
- **Pinterest** — pins, short links, boards via Pinterest API (no pinterest-dl)
- **Pinterest HTTP API** — `POST /download/pinterest` (when `DOWNLOAD_API_ENABLED=true`)
- **TikTok** — video + carousel slideshows via yt-dlp + image download
- **TikTok carousel button** — download photos from video posts (`ttk:img:` callback)
- **Spotify** — release cards via Spotify API; optional audio via YouTube search + yt-dlp
- **SoundCloud** — metadata via yt-dlp; optional audio download
- **Download cancel** — cancel button + active download queue (`dlc:` / `dlq:` callbacks)
- **Admin panel** — `/admin` runtime settings (Redis hot-swap), shadow bans, user stats
- **Broadcasts** — `/broadcast` create/send with asynq worker
- **Runtime settings** — global + all platform int/bool keys (YouTube enum/list deferred)
- Link parsing (YouTube, TikTok, Instagram, X, Pinterest, Spotify, SoundCloud)
- Video/image downloads via `yt-dlp` subprocess
- Rate limiting + per-user download lock (Redis)
- Prometheus metrics on `:9101`
- Shared PostgreSQL schema (existing Alembic migrations)

## Not yet migrated

- YouTube enum/list runtime settings (quality/ratio lists via admin)
- Pinterest HTTP API auth (same as Python: no auth, disable via env)

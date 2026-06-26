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

## Migrated (phase 1–3)

- Webhook + polling
- `/start` onboarding (EN/RU)
- `/settings` — language, default YouTube quality/ratio
- YouTube quality + aspect ratio picker with ffmpeg transcoding
- **Pinterest** — pins, short links, boards via Pinterest API (no pinterest-dl)
- **TikTok** — video + carousel slideshows via yt-dlp + image download
- Link parsing (YouTube, TikTok, Instagram, X, Pinterest, Spotify*, SoundCloud*)
- Video/image downloads via `yt-dlp` subprocess
- Rate limiting + per-user download lock (Redis)
- Prometheus metrics on `:9101`
- Shared PostgreSQL schema (existing Alembic migrations)

## Not yet migrated

- TikTok carousel button (download images from video post)
- Spotify / SoundCloud metadata + audio
- Admin panel, broadcasts, download cancel
- Pinterest HTTP API

\* Spotify/SoundCloud links are recognized but return a placeholder response until phase 3.

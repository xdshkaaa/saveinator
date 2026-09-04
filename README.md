# Saveinator

Telegram bots for downloading media from YouTube, TikTok, X/Twitter, Instagram, Reddit, Spotify, SoundCloud, Yandex Music, and Pinterest.

Production runs as **Go binaries**: the main `saveinator` bot (webhook/polling + asynq worker + Prometheus metrics), a `botd` fleet process that hosts every secondary bot from [`bots.yaml`](bots.yaml), and the `dash` operator dashboard. See [`go/README.md`](go/README.md) for development details.

## Features

- Main `saveinator` bot: YouTube (quality/aspect-ratio picker, transcoding), TikTok (video + carousel slideshows), X/Twitter (video + photo fallback), Instagram, Yandex Music, Spotify and SoundCloud metadata with optional audio via yt-dlp
- Reddit: video/galleries/GIFs via yt-dlp (cookie-authenticated — Reddit requires login) + full thread articles on Telegraph with comments and a one-tap RU translate button (requires `secrets/reddit_cookies.txt`, see [`secrets/README.md`](secrets/README.md))
- Fleet bots via `botd` (one process, one config): Pinterest EN/RU, Pinterest KZ (Kazakh), SoundCloud, Spotify — each with its own token and locale
- Pinterest pins, boards, and short links; HTTP API `POST /download/pinterest` (guarded by `X-Internal-Token`)
- `dash` operator dashboard: service probes, aggregate stats, full user table, admin URL test-runner (`test_urls`, executed by the saveinator worker); Telegram Login auth
- EN/RU/KK localization, admin panel, broadcasts, `/clear` queue command, download cancel
- Runtime settings in Redis (hot-swap), rate limiting, per-user download locks, group anti-spam
- Prometheus metrics + Grafana dashboards — see [`MONITORING.md`](docs/MONITORING.md)

## Quick start (production Docker)

```bash
cp .env.example .env
# Edit BOT_TOKEN, the four fleet bot tokens, DB_PASSWORD, INTERNAL_API_TOKEN, etc.

docker compose up -d --build
docker compose --profile tools run --rm migrate   # first deploy / schema updates
```

Services:

| Service | Description |
|---------|-------------|
| `saveinator` | Main Go app: Telegram webhook `:8000`, asynq worker, metrics `:9101` + `:9102` |
| `botd` | Fleet from `bots.yaml`: one webhook `:8005` with `/webhook/{slug}` per bot, metrics `:9106`, download API |
| `dash` | Operator dashboard (host network): `127.0.0.1:9000`, Telegram Login auth, service probes |
| `db` | PostgreSQL 16 |
| `redis` | Queue, locks, rate limits, runtime settings (shared) |
| `migrate` | Alembic migrations (`--profile tools`) |

## Local development

```bash
docker compose -f docker-compose.dev.yml up -d   # Postgres + Redis
cd go && go run ./cmd/saveinator                 # main bot
cd go && go run ./cmd/botd                       # fleet bots (BOTS_CONFIG=bots.yaml)
```

Set `USE_POLLING=true` for local polling without a webhook.

## Production webhook

Main bot:

```env
USE_POLLING=false
WEBHOOK_HOST=https://<webhook-host>   # public HTTPS host behind Caddy/Cloudflare
WEBHOOK_PATH=/webhook
WEBHOOK_SECRET_TOKEN=long-random-value
```

Fleet bots (`botd`, same host, single port):

```env
BOTD_USE_POLLING=false
BOTD_WEBHOOK_HOST=https://<webhook-host>
BOTD_WEBHOOK_PORT=8005
```

Public routes (via Caddy/Cloudflare): `/webhook`, `/webhook/{slug}`, `/download/pinterest`, `/health`. Metrics stay on loopback (`:9101`, `:9102`, `:9106`).

## Deploy to VPS

```bash
VPS_HOST=<host> ./scripts/deploy.sh   # sync + build saveinator/botd/dash + migrate + systemd unit
```

`dash` can be deployed/reconfigured separately with `scripts/deploy-dash.sh` (patches the shared Caddy/Cloudflared configs). Legacy one-shot migration scripts (post Python cutover): `scripts/cutover-to-go.sh`, `scripts/cleanup-python-vps.sh`.

Monitoring: see [`MONITORING.md`](docs/MONITORING.md). Pinterest internals: [`PINTEREST_DOWNLOADER.md`](docs/PINTEREST_DOWNLOADER.md).

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
| `PINTEREST_BOT_TOKEN` / `PINTEREST_KZ_BOT_TOKEN` | Fleet bot tokens (required by `botd`) |
| `SPOTIFY_BOT_TOKEN` / `SOUNDCLOUD_BOT_TOKEN` | Fleet bot tokens (required by `botd`) |
| `DATABASE_URL` | PostgreSQL URL (`postgresql+asyncpg://...` in Docker) |
| `DB_PASSWORD` | Postgres password for Compose |
| `REDIS_URL` | Redis for asynq and rate limits |
| `SAVEINATOR_MODE` | `all` (default), `bot`, or `worker` |
| `INTERNAL_API_TOKEN` | Shared secret (`X-Internal-Token`) for `/download/pinterest` |
| `BOTS_CONFIG` | Path to `bots.yaml` for `botd` |
| `DASH_TELEGRAM_TOKEN` / `DASH_ADMIN_IDS` | Dash Telegram Login token + operator id allowlist |
| `METRICS_PORT` / `WORKER_METRICS_PORT` | Main bot Prometheus ports (default `9101` / `9102`; `botd` uses `9106`) |

## Architecture

```text
Telegram → Caddy/Cloudflare → saveinator :8000  (/webhook)
Telegram → Caddy/Cloudflare → botd       :8005  (/webhook/{slug})
HTTP client → Caddy/Cloudflare → botd    :8005  (/download/pinterest, X-Internal-Token)
saveinator / botd → PostgreSQL (users, downloads, broadcasts, test_urls)
saveinator / botd → Redis (asynq queue, locks, runtime settings)
saveinator / botd → yt-dlp / ffmpeg subprocesses
dash :9000 → Postgres/Redis (read-only + test_urls), probes app & monitoring endpoints on loopback
Prometheus → :9101 + :9102 + :9106 → Grafana
```

Code layout:

- [`go/`](go/) — application source (`cmd/saveinator`, `cmd/botd`, `cmd/dash`, `internal/…`)
- [`bots.yaml`](bots.yaml) — fleet config for `botd` (one block per bot, no code, no ports)
- [`go/internal/locale/locales/`](go/internal/locale/locales/) — EN/RU/KK strings
- [`db/migrations/`](db/migrations/) — Alembic schema history
- [`monitoring/`](monitoring/) — Prometheus, Grafana, alerts, Caddy/Cloudflared configs
- [`docker/`](docker/) — Dockerfiles (`go`, `botd`, `dash`, `migrate`)
- [`systemd/`](systemd/) — `ytbot.service` unit used by the deploy script

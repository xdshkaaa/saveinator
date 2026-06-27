# save-yt-tiktok (Saveinator)

Telegram bot for downloading videos from YouTube, TikTok, Instagram, and X/Twitter, Pinterest images/videos, Spotify album/single/track metadata and audio downloads via yt-dlp YouTube search, plus SoundCloud track/playlist metadata and audio downloads via yt-dlp.

## Go rewrite (recommended for production)

A single Go binary replaces the Python `bot` + Celery `worker` stack (~30–50 MB RAM vs ~300–600 MB). See [`go/README.md`](go/README.md).

```bash
docker compose -f docker-compose.go.yml up -d --build
```

Set `SAVEINATOR_MODE=all` (default) for webhook/polling + asynq worker + metrics in one container.

The sections below describe the legacy Python/Celery deployment.

## Features

- Download videos via yt-dlp (YouTube, TikTok, Instagram, X/Twitter)
- Download Pinterest pins and boards via [`pinterest-dl`](https://github.com/sean1832/pinterest-dl)
- Show Spotify album/single/compilation/track metadata from the Spotify Web API
- Download Spotify album/track audio via yt-dlp YouTube search (not Spotify streaming)
- Show SoundCloud track/playlist metadata via yt-dlp
- Download SoundCloud track/playlist audio via yt-dlp (public content only)
- `POST /download/pinterest` HTTP API for programmatic downloads
- EN/RU localization
- Celery workers for background video downloads

## Spotify support

Spotify metadata comes from the Spotify Web API. Audio downloads match tracks on YouTube via yt-dlp — the bot does **not** stream or rip audio directly from Spotify.

### Supported links

- `https://open.spotify.com/album/{id}`
- `https://open.spotify.com/track/{id}`
- `spotify:album:{id}`
- `spotify:track:{id}`
- URLs with query parameters (for example `?si=...`) — the ID is extracted after stripping query params

### What happens on a Spotify link

1. The bot parses the URL/URI and validates the Spotify ID.
2. Metadata is fetched via Spotify Client Credentials API.
3. Data is normalized into internal release/track models.
4. The user receives a metadata card with cover art (when available) and an **Open in Spotify** button.
5. If `SPOTIFY_DOWNLOAD_ENABLED=true`, tracks are downloaded in the background and sent as audio files.

**Album / single / compilation card example:**

```text
🎵 Artist — Release name
Type: single / album / compilation
Tracks: N
Release date: YYYY-MM-DD
```

**Track card example:**

```text
🎵 Artist — Track name
Duration: M:SS
```

If download is disabled (`SPOTIFY_DOWNLOAD_ENABLED=false`), the bot replies with a metadata-only message.

### Audio download

```text
Spotify metadata → yt-dlp ytsearch → audio files → Telegram
```

- Per-track timeout: `SPOTIFY_TRACK_TIMEOUT_SECONDS` (default `15`).
- Requires `ffmpeg` in the bot container.
- YouTube matching quality depends on track availability — some items may fail.

### Enable Spotify metadata

1. Create a Spotify app in the [Spotify Developer Dashboard](https://developer.spotify.com/dashboard)
2. Set credentials in `.env`
3. Enable the feature flag: `SPOTIFY_ENABLED=true`

## SoundCloud support

SoundCloud metadata and audio downloads use yt-dlp directly on public SoundCloud URLs. The bot does **not** bypass private tracks, authentication, or DRM.

### Supported links

- `https://soundcloud.com/{artist}/{track-name}`
- `https://soundcloud.com/{artist}/sets/{playlist-name}`
- `https://on.soundcloud.com/{short-code}`
- URLs with query parameters — query/hash are stripped before parsing

### What happens on a SoundCloud link

1. The bot parses the URL and detects track, playlist, or short link.
2. Metadata is fetched via yt-dlp in metadata-only mode.
3. Data is normalized into internal track/release models.
4. The user receives a metadata card with artwork (when available) and an **Open in SoundCloud** button.
5. If `SOUNDCLOUD_DOWNLOAD_ENABLED=true`, tracks are downloaded in the background and sent as audio files.

**Track card example:**

```text
🎧 Artist — Track title
Duration: 3:42
Genre: Electronic
Source: SoundCloud
```

**Playlist card example:**

```text
🎧 Artist — Playlist title
Tracks: 12
Source: SoundCloud
```

If download is disabled (`SOUNDCLOUD_DOWNLOAD_ENABLED=false`), the bot replies with a metadata-only message.

### Enable SoundCloud

```env
SOUNDCLOUD_ENABLED=true
SOUNDCLOUD_DOWNLOAD_ENABLED=false
SOUNDCLOUD_TRACK_TIMEOUT_SECONDS=30
SOUNDCLOUD_MAX_TRACKS=20
SOUNDCLOUD_DL_OUTPUT_FORMAT=mp3
SOUNDCLOUD_MAX_FILE_MB=50
SOUNDCLOUD_META_CACHE_TTL_SECONDS=3600
```

## Setup

```bash
cd save-yt-tiktok
cp .env.example .env
uv sync --extra dev
```

Fill in at minimum:

```env
BOT_TOKEN=replace_with_telegram_bot_token
```

For Spotify metadata cards:

```env
SPOTIFY_ENABLED=true
SPOTIFY_CLIENT_ID=your-spotify-client-id
SPOTIFY_CLIENT_SECRET=<spotify-client-secret>
SPOTIFY_API_TIMEOUT_SECONDS=15
SPOTIFY_DOWNLOAD_ENABLED=true
SPOTIFY_TRACK_TIMEOUT_SECONDS=15
SPOTIFY_DL_OUTPUT_FORMAT=mp3
```

## Run locally

```bash
# terminal 1: Redis (or use docker compose -f docker-compose.dev.yml up -d)
redis-server

# terminal 2: Celery worker
uv run celery -A workers.app worker --loglevel=info

# terminal 3: bot (polling)
USE_POLLING=true uv run python -m bot.main
```

For the local Postgres/Redis helper stack:

```bash
docker compose -f docker-compose.dev.yml up -d
```

## Production webhook

Production Docker defaults to Telegram webhook mode. The public webhook host is:

```env
USE_POLLING=false
WEBHOOK_HOST=https://saveinator-hooks.xdshka.party
WEBHOOK_PATH=/webhook
WEBHOOK_SECRET_TOKEN=long-random-value
```

The webhook app also serves `GET /health` for origin checks. Keep `/metrics` private on `127.0.0.1:9101`; the Caddy/Cloudflare route for `saveinator-hooks.xdshka.party` only forwards `/`, `/health`, and `/webhook*`.

## Tests

```bash
uv run pytest -q
```

The repository also runs the same test command in GitHub Actions.

## Environment variables

| Variable | Default | Description |
|----------|---------|-------------|
| `BOT_TOKEN` | required | Telegram bot token |
| `DATABASE_URL` | SQLite dev DB | Async SQLAlchemy URL |
| `DB_PASSWORD` | required for Docker | PostgreSQL password used by the production Compose stack |
| `REDIS_URL` | `redis://localhost:6379/0` | Rate limit / spam dedup |
| `USE_POLLING` | `true` locally / `false` in Docker | Polling vs webhook mode |
| `WEBHOOK_HOST` | `https://saveinator-hooks.xdshka.party` | Public Telegram webhook origin |
| `WEBHOOK_PATH` | `/webhook` | Telegram webhook path |
| `WEBHOOK_SECRET_TOKEN` | `""` | Optional Telegram webhook secret header validation |
| `SPOTIFY_ENABLED` | `false` | Enable Spotify metadata cards |
| `SPOTIFY_CLIENT_ID` | `""` | Spotify API client ID |
| `SPOTIFY_CLIENT_SECRET` | `""` | Spotify API client secret |
| `SPOTIFY_API_TIMEOUT_SECONDS` | `15` | Spotify HTTP timeout |
| `SPOTIFY_DOWNLOAD_ENABLED` | `true` | Enable Spotify audio downloads |
| `SPOTIFY_TRACK_TIMEOUT_SECONDS` | `15` | Per-track yt-dlp timeout |
| `SPOTIFY_DL_OUTPUT_FORMAT` | `mp3` | Output format (`mp3`, `flac`, `wav`, `aac`) |
| `SOUNDCLOUD_ENABLED` | `true` | Enable SoundCloud metadata cards |
| `SOUNDCLOUD_DOWNLOAD_ENABLED` | `false` | Enable SoundCloud audio downloads |
| `SOUNDCLOUD_TRACK_TIMEOUT_SECONDS` | `30` | Per-track yt-dlp timeout |
| `SOUNDCLOUD_MAX_TRACKS` | `20` | Maximum playlist tracks to process |
| `SOUNDCLOUD_DL_OUTPUT_FORMAT` | `mp3` | Output format |
| `SOUNDCLOUD_MAX_FILE_MB` | `50` | Maximum audio file size |
| `SOUNDCLOUD_META_CACHE_TTL_SECONDS` | `3600` | Metadata cache TTL |

See [`.env.example`](.env.example) for the full list.

## Pinterest support

Pinterest pins, short links (`pin.it`), and boards are downloaded in the Celery worker via `pinterest-dl`.

- Telegram: send a Pinterest URL to the bot
- HTTP API: `POST /download/pinterest` (see [PINTEREST_DOWNLOADER.md](PINTEREST_DOWNLOADER.md))
- Default mode uses Pinterest API scraping (no browser required)
- Optional browser mode: `PINTEREST_USE_BROWSER=true`
- Private pins: set `PINTEREST_COOKIES_PATH` to an authorized cookies file

## Architecture notes

- Video downloads: `bot/handlers/group.py` → Celery `download_and_send_task` → `workers/downloader.py` (yt-dlp)
- Pinterest downloads: `bot/handlers/group.py` → Celery `pinterest_download_task` → `workers/pinterest_downloader.py`
- Spotify and SoundCloud audio downloads are matched through yt-dlp and sent through shared audio cover/file sender helpers.

## Repository hygiene

- Keep real `.env`, `.env.monitoring`, local databases, caches, generated repo bundles, and `.commandcode/` notes out of git.
- Commit changes with `uv.lock` when dependency resolution changes.
- Run `uv run pytest -q` before opening a pull request or deploying.
- Pinterest HTTP API: `bot/api/pinterest.py`
- Spotify metadata: `bot/handlers/group.py` → `bot/services/spotify_handler.py` → `bot/services/spotify_client.py`
- Spotify audio download: `bot/services/spotify_handler.py` → `bot/services/youtube_audio.py` (yt-dlp)
- Spotify URL parsing: `bot/services/spotify_parser.py`
- SoundCloud metadata: `bot/handlers/group.py` → `bot/services/soundcloud_handler.py` → `bot/services/soundcloud_client.py` (yt-dlp)
- SoundCloud audio download: `bot/services/soundcloud_handler.py` → `bot/services/soundcloud_audio.py` (yt-dlp)
- SoundCloud URL parsing: `bot/services/soundcloud_parser.py`
- Feature flags: `SPOTIFY_ENABLED`, `SOUNDCLOUD_ENABLED`, `PINTEREST_ENABLED`

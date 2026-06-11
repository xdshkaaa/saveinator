# save-yt-tiktok (Saveinator)

Telegram bot for downloading videos from YouTube, TikTok, Instagram, and X/Twitter, Pinterest images/videos, plus Spotify album/single/track metadata and audio downloads via yt-dlp YouTube search.

## Features

- Download videos via yt-dlp (YouTube, TikTok, Instagram, X/Twitter)
- Download Pinterest pins and boards via [`pinterest-dl`](https://github.com/sean1832/pinterest-dl)
- Show Spotify album/single/compilation/track metadata from the Spotify Web API
- Download Spotify album/track audio via yt-dlp YouTube search (not Spotify streaming)
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

## Setup

```bash
cd save-yt-tiktok
cp .env.example .env
pip install -e ".[dev]"
```

Fill in at minimum:

```env
BOT_TOKEN=your-telegram-bot-token
```

For Spotify metadata cards:

```env
SPOTIFY_ENABLED=true
SPOTIFY_CLIENT_ID=your-spotify-client-id
SPOTIFY_CLIENT_SECRET=your-spotify-client-secret
SPOTIFY_API_TIMEOUT_SECONDS=15
SPOTIFY_DOWNLOAD_ENABLED=true
SPOTIFY_TRACK_TIMEOUT_SECONDS=15
SPOTIFY_DL_OUTPUT_FORMAT=mp3
```

## Run locally

```bash
# terminal 1: Redis
redis-server

# terminal 2: Celery worker
celery -A workers.app worker --loglevel=info

# terminal 3: bot (polling)
USE_POLLING=true python -m bot.main
```

## Tests

```bash
pytest tests/test_spotify_parser.py tests/test_youtube_audio.py tests/test_link_parser.py tests/test_spotify_client.py tests/test_spotify_handler.py -q
```

## Environment variables

| Variable | Default | Description |
|----------|---------|-------------|
| `BOT_TOKEN` | required | Telegram bot token |
| `DATABASE_URL` | SQLite dev DB | Async SQLAlchemy URL |
| `REDIS_URL` | `redis://localhost:6379/0` | Rate limit / spam dedup |
| `USE_POLLING` | `true` | Polling vs webhook mode |
| `SPOTIFY_ENABLED` | `false` | Enable Spotify metadata cards |
| `SPOTIFY_CLIENT_ID` | `""` | Spotify API client ID |
| `SPOTIFY_CLIENT_SECRET` | `""` | Spotify API client secret |
| `SPOTIFY_API_TIMEOUT_SECONDS` | `15` | Spotify HTTP timeout |
| `SPOTIFY_DOWNLOAD_ENABLED` | `true` | Enable Spotify audio downloads |
| `SPOTIFY_TRACK_TIMEOUT_SECONDS` | `15` | Per-track yt-dlp timeout |
| `SPOTIFY_DL_OUTPUT_FORMAT` | `mp3` | Output format (`mp3`, `flac`, `wav`, `aac`) |

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
- Pinterest HTTP API: `bot/api/pinterest.py`
- Spotify metadata: `bot/handlers/group.py` → `bot/services/spotify_handler.py` → `bot/services/spotify_client.py`
- Spotify audio download: `bot/services/spotify_handler.py` → `bot/services/youtube_audio.py` (yt-dlp)
- Spotify URL parsing: `bot/services/spotify_parser.py`
- Feature flags: `SPOTIFY_ENABLED`, `PINTEREST_ENABLED`

# save-yt-tiktok (Saveinator)

Telegram bot for downloading videos from YouTube, TikTok, Instagram, and X/Twitter, plus Spotify album/single/track metadata and audio downloads via [spotify-dl](https://github.com/SwapnilSoni1999/spotify-dl).

## Features

- Download videos via yt-dlp (YouTube, TikTok, Instagram, X/Twitter)
- Show Spotify album/single/compilation/track metadata from the Spotify Web API
- Download Spotify album/track audio via spotify-dl (YouTube matching, not Spotify streaming)
- EN/RU localization
- Celery workers for background video downloads

## Spotify support

Spotify metadata comes from the Spotify Web API. Audio downloads use [spotify-dl](https://github.com/SwapnilSoni1999/spotify-dl), which matches tracks on YouTube — the bot does **not** stream or rip audio directly from Spotify.

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
5. If `SPOTIFY_DOWNLOAD_ENABLED=true` and spotify-dl is installed, tracks are downloaded in the background and sent as audio files.

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

If download is disabled (`SPOTIFY_DOWNLOAD_ENABLED=false` or spotify-dl is missing), the bot replies with a metadata-only message.

### Audio download via spotify-dl

```text
Spotify metadata card → spotify-dl CLI → YouTube match → audio files → Telegram
```

- Uses your `SPOTIFY_CLIENT_ID` / `SPOTIFY_CLIENT_SECRET` as `--ak` for spotify-dl.
- Requires `ffmpeg` and Node.js (installed in the bot Docker image).
- Albums may take several minutes; timeout is controlled by `SPOTIFY_DL_TIMEOUT_SECONDS`.
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
SPOTIFY_API_TIMEOUT_SECONDS=10
SPOTIFY_DOWNLOAD_ENABLED=true
SPOTIFY_DL_TIMEOUT_SECONDS=600
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
pytest tests/test_spotify_parser.py tests/test_spotify_dl.py tests/test_link_parser.py tests/test_spotify_client.py tests/test_spotify_handler.py -q
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
| `SPOTIFY_API_TIMEOUT_SECONDS` | `10` | Spotify HTTP timeout |
| `SPOTIFY_DOWNLOAD_ENABLED` | `true` | Enable spotify-dl audio downloads |
| `SPOTIFY_DL_TIMEOUT_SECONDS` | `600` | spotify-dl subprocess timeout |
| `SPOTIFY_DL_OUTPUT_FORMAT` | `mp3` | Output format (`mp3`, `flac`, `wav`, `aac`) |

See [`.env.example`](.env.example) for the full list.

## Architecture notes

- Video downloads: `bot/handlers/group.py` → Celery `download_and_send_task` → `workers/downloader.py` (yt-dlp)
- Spotify metadata: `bot/handlers/group.py` → `bot/services/spotify_handler.py` → `bot/services/spotify_client.py`
- Spotify audio download: `bot/services/spotify_handler.py` → `bot/services/spotify_dl.py` (spotify-dl CLI)
- Spotify URL parsing: `bot/services/spotify_parser.py`
- Feature flag: `SPOTIFY_ENABLED`

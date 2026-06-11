# save-yt-tiktok (Saveinator)

Telegram bot for downloading videos from YouTube, TikTok, Instagram, and X/Twitter, plus Spotify album/single/track metadata cards.

## Features

- Download videos via yt-dlp (YouTube, TikTok, Instagram, X/Twitter)
- Show Spotify album/single/compilation/track metadata from the Spotify Web API
- EN/RU localization
- Celery workers for background video downloads

## Spotify support

Spotify integration uses Spotify only for metadata. The bot does **not** download audio from Spotify or bypass Spotify restrictions.

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

If no optional audio search/download providers are configured, the bot also replies:

```text
For Spotify, only the release card and track list are available. Downloading Spotify content is not supported.
```

### Optional search/download providers

Spotify metadata can be connected to an external audio pipeline through provider abstractions in `bot/services/audio_providers.py`:

```text
Spotify metadata → AudioSearchProvider → AudioDownloadProvider
```

- Spotify module is metadata-only; it builds search queries like `Artist - Track`.
- Video download flow (`yt-dlp`) stays separate from Spotify.
- Register providers at deployment time via `register_audio_providers(search, download)`.
- The deployment owner is responsible for choosing legal/permitted audio sources.

By default, no search/download providers are registered — only metadata cards are shown.

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
pytest tests/test_spotify_parser.py tests/test_link_parser.py tests/test_spotify_client.py tests/test_spotify_handler.py -q
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

See [`.env.example`](.env.example) for the full list.

## Architecture notes

- Video downloads: `bot/handlers/group.py` → Celery `download_and_send_task` → `workers/downloader.py` (yt-dlp)
- Spotify metadata: `bot/handlers/group.py` → `bot/services/spotify_handler.py` → `bot/services/spotify_client.py` (Client Credentials, no audio download)
- Spotify URL parsing: `bot/services/spotify_parser.py`
- Optional audio pipeline: `bot/services/audio_providers.py`
- Feature flag: `SPOTIFY_ENABLED`

# save-yt-tiktok (Saveinator)

Telegram bot for downloading videos from YouTube, TikTok, Instagram, and X/Twitter, plus Spotify album/single metadata cards.

## Features

- Download videos via yt-dlp (YouTube, TikTok, Instagram, X/Twitter)
- Show Spotify album/single/compilation metadata from the Spotify Web API
- EN/RU localization
- Celery workers for background downloads

## Spotify support

The bot recognizes Spotify album links:

- `https://open.spotify.com/album/{id}`
- `spotify:album:{id}`

When a user sends a Spotify album link, the bot replies with a metadata card:

- release name, artist, type (`album` / `single` / `compilation`)
- track count and release date
- cover art (when available)
- **Open in Spotify** button

**Important:** the bot does **not** download or stream audio from Spotify. It does not bypass Spotify DRM, licensing, or Terms of Service. Only public metadata from the Spotify Web API is shown.

To enable Spotify metadata:

1. Create a Spotify app in the [Spotify Developer Dashboard](https://developer.spotify.com/dashboard)
2. Set Client Credentials (`SPOTIFY_CLIENT_ID`, `SPOTIFY_CLIENT_SECRET`)
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
pytest tests/test_link_parser.py tests/test_spotify_client.py tests/test_spotify_handler.py -q
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
- Spotify metadata: inline async handler branch → `bot/services/spotify_client.py` (Client Credentials, no audio download)
- Feature flag: `SPOTIFY_ENABLED`

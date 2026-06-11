# Pinterest Downloader

Saveinator downloads Pinterest images and video streams using the unofficial [`pinterest-dl`](https://github.com/sean1832/pinterest-dl) library.

## Supported URLs

| Type | Example |
|------|---------|
| Pin | `https://www.pinterest.com/pin/123456789/` |
| Short link | `https://pin.it/abc123` |
| Board | `https://www.pinterest.com/username/board-name/` |

Private pins are not downloaded unless you provide authorized cookies (see below).

## Architecture

```
Telegram message / HTTP API
        │
        ▼
bot/services/pinterest_parser.py   # URL validation
        │
        ├── Telegram: workers/pinterest_task.py (Celery)
        └── HTTP API: bot/api/pinterest.py
        │
        ▼
workers/pinterest_downloader.py    # pinterest-dl adapter
        │
        ▼
/tmp/ytbot/{task_id}/              # ephemeral storage
```

### Modules

| Module | Purpose |
|--------|---------|
| `bot/services/pinterest_parser.py` | Pin/board/short URL detection |
| `bot/services/pinterest_models.py` | `PinterestMediaItem`, `PinterestDownloadResult` |
| `workers/pinterest_downloader.py` | Download adapter around `pinterest-dl` |
| `workers/pinterest_task.py` | Celery task for Telegram delivery |
| `bot/api/pinterest.py` | `POST /download/pinterest` HTTP endpoint |

## Telegram usage

Send any supported Pinterest URL to the bot in a private or group chat. The bot enqueues `pinterest_download_task` and sends each downloaded image/video back to the chat.

## HTTP API

Available on:

- Webhook mode: `http://{host}:{WEBHOOK_PORT}/download/pinterest`
- Polling mode: `http://127.0.0.1:{METRICS_PORT}/download/pinterest`

Disable with `DOWNLOAD_API_ENABLED=false`.

### Request

```http
POST /download/pinterest
Content-Type: application/json
```

```json
{
  "url": "https://www.pinterest.com/pin/123456789/",
  "limit": 10,
  "downloadVideos": true,
  "downloadImages": true
}
```

### Response

```json
{
  "url": "https://www.pinterest.com/pin/123456789/",
  "url_type": "pin",
  "count": 2,
  "items": [
    {
      "source_url": "https://www.pinterest.com/pin/123456789/",
      "media_type": "image",
      "title": "Sunset photo",
      "description": "Sunset photo",
      "original_media_url": "https://i.pinimg.com/originals/...",
      "file_path": "/tmp/ytbot/uuid/photo.jpg",
      "file_size": 245760,
      "created_at": "2026-06-11T12:00:00Z"
    }
  ],
  "errors": []
}
```

### Error codes

| Status | Meaning |
|--------|---------|
| `400` | Invalid JSON or unsupported Pinterest URL |
| `403` | Private content / unauthorized access |
| `404` | No media found |
| `422` | Download rejected by pinterest-dl |
| `503` | `PINTEREST_ENABLED=false` |
| `504` | Exceeded `PINTEREST_TIMEOUT_SECONDS` |

## Metadata fields

Each downloaded item includes:

- `source_url` — original Pinterest URL submitted
- `media_type` — `image` or `video`
- `title` / `description` — from Pinterest alt text when available
- `original_media_url` — direct image or video stream URL
- `file_path` — local path under `/tmp/ytbot/`
- `file_size` — bytes
- `created_at` — file mtime UTC

## Environment variables

| Variable | Default | Description |
|----------|---------|-------------|
| `PINTEREST_ENABLED` | `true` | Enable Pinterest in bot + API |
| `PINTEREST_TIMEOUT_SECONDS` | `30` | Overall download timeout |
| `PINTEREST_MAX_ITEMS` | `10` | Max pins/media per URL (Telegram) |
| `PINTEREST_DOWNLOAD_IMAGES` | `true` | Download image pins |
| `PINTEREST_DOWNLOAD_VIDEOS` | `true` | Download video streams |
| `PINTEREST_API_TIMEOUT_SECONDS` | `10` | pinterest-dl HTTP timeout |
| `PINTEREST_USE_BROWSER` | `false` | Use browser mode instead of API mode |
| `PINTEREST_COOKIES_PATH` | `""` | Optional Netscape cookies file for private pins |
| `PINTEREST_SAVE_METADATA` | `true` | Save sidecar metadata via pinterest-dl |
| `DOWNLOAD_API_ENABLED` | `true` | Expose `POST /download/pinterest` |

## Browser / Selenium mode

By default the worker uses `PinterestDL.with_api()` — no Chrome/Firefox required.

Set `PINTEREST_USE_BROWSER=true` to switch to `PinterestDL.with_browser()`. This requires a browser runtime in the worker container. The stock Docker image does **not** include Chrome; add it only if API mode is insufficient.

## Authorized cookies (optional)

To access private boards or pins you own:

1. Export browser cookies to a Netscape-format file
2. Mount it into the worker container
3. Set `PINTEREST_COOKIES_PATH=/path/to/cookies.txt`

Never commit cookie files or account credentials to git.

## Tests

```bash
pytest tests/test_pinterest_parser.py \
       tests/test_pinterest_downloader.py \
       tests/test_pinterest_task.py \
       tests/test_pinterest_api.py -q
```

## Troubleshooting

| Problem | Fix |
|---------|-----|
| `No media found` | Pin may be deleted, private, or image-only with `downloadVideos=false` |
| `private or requires authorized cookies` | Provide `PINTEREST_COOKIES_PATH` |
| API timeout | Increase `PINTEREST_TIMEOUT_SECONDS` |
| Board returns few items | Lower `limit` for API; boards respect `num` parameter |

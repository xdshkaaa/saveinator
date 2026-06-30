---
name: saveinator-pinterest
description: >-
  Pinterest downloads in Saveinator Go — pins, boards, short links, HTTP API.
  Use when Pinterest pin, board, pin.it, POST /download/pinterest, or PINTEREST_* env vars.
---

# Saveinator Pinterest (Go)

Pinterest is implemented entirely in Go — **not** `pinterest-dl` or legacy Python workers.

## Supported URLs

| Type | Example |
|------|---------|
| Pin | `https://www.pinterest.com/pin/123456789/` |
| Short link | `https://pin.it/abc123` |
| Board | `https://www.pinterest.com/username/board-name/` |

## Architecture

```
Telegram message / HTTP API
        │
        ▼
go/internal/linkparser/parser.go     # URL validation
        │
        ├── Telegram: handler → queue → worker/pinterest_tiktok.go
        └── HTTP API: go/internal/api/pinterest.go
        │
        ▼
go/internal/pinterest/client.go      # PinResource / BoardFeedResource API
        │
        ▼
/tmp/saveinator-*/                   # ephemeral storage
```

### Key modules

| Module | Purpose |
|--------|---------|
| `go/internal/pinterest/parser.go` | Pin/board/short URL detection |
| `go/internal/pinterest/client.go` | Pinterest API + file download |
| `go/internal/pinterest/json.go` | Response parsing |
| `go/internal/worker/pinterest_tiktok.go` | asynq worker handler |
| `go/internal/api/pinterest.go` | `POST /download/pinterest` |

Pins and short links use `PinResource` API. Boards use `BoardFeedResource` API.

## Telegram usage

Send a supported Pinterest URL to the bot. Handler enqueues `download:pinterest` task; worker sends images/videos back to chat.

## HTTP API

When `DOWNLOAD_API_ENABLED=true`:

- Webhook mode: `http://{host}:{WEBHOOK_PORT}/download/pinterest`
- Polling mode: `http://127.0.0.1:{METRICS_PORT}/download/pinterest`

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

### Error codes

| Status | Meaning |
|--------|---------|
| `400` | Invalid JSON or unsupported URL |
| `403` | Private content / unauthorized |
| `404` | No media found |
| `503` | `PINTEREST_ENABLED=false` |
| `504` | Exceeded `PINTEREST_TIMEOUT_SECONDS` |

## Environment variables (Go)

| Variable | Default | Description |
|----------|---------|-------------|
| `PINTEREST_ENABLED` | `true` | Enable Pinterest in bot + API |
| `PINTEREST_TIMEOUT_SECONDS` | `30` | Overall download timeout |
| `PINTEREST_MAX_ITEMS` | `10` | Max items per URL (`.env.example` may show `1`) |
| `PINTEREST_DOWNLOAD_IMAGES` | `true` | Download image pins |
| `PINTEREST_DOWNLOAD_VIDEOS` | `true` | Download video streams |
| `PINTEREST_COOKIES_PATH` | `""` | Optional Netscape cookies for private pins |
| `DOWNLOAD_API_ENABLED` | `true` | Expose HTTP API |

**Not used in Go** (legacy doc only): `PINTEREST_USE_BROWSER`, `PINTEREST_API_TIMEOUT_SECONDS`, `PINTEREST_SAVE_METADATA`.

## Private pins

1. Export browser cookies to Netscape format
2. Mount via `PINTEREST_COOKIES_PATH` (see `saveinator-cookies`)
3. Never commit cookie files

## Tests

```bash
cd go && go test ./internal/pinterest/... ./internal/api/...
```

## Troubleshooting

| Problem | Fix |
|---------|-----|
| `No media found` | Pin deleted, private, or `downloadVideos=false` on video-only pin |
| Private pin | Set `PINTEREST_COOKIES_PATH` |
| API timeout | Increase `PINTEREST_TIMEOUT_SECONDS` |
| Board returns few items | Check `limit` / `PINTEREST_MAX_ITEMS` |

## Related

- New platform scaffold: `saveinator-handler-worker-scaffold`
- Cookie setup: `saveinator-cookies`

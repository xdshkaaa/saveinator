---
name: saveinator-handler-worker-scaffold
description: >-
  Scaffolds a new Saveinator download flow from URL parsing through asynq worker to Telegram
  reply. Use when adding a platform, callback handler, enqueue path, or download feature in go/.
---

# Saveinator Handler → Worker Scaffold

Go download flows split across handler (Telegram + enqueue) and worker (yt-dlp / API / send).

## Pipeline (follow in order)

```
1. linkparser/parser.go     — PlatformX constant + regex
2. linkparser/parser_test.go — table-driven cases (mirror tests/test_link_parser.py)
3. handler/bot.go           — dispatchLink() case + runtime.PlatformEnabled check
4. handler/<platform>.go    — sync UI OR session before enqueue (if needed)
5. queue/client.go          — TypeX constant + EnqueueX() + payload struct fields
6. worker/<platform>.go     — handleX(ctx, *asynq.Task)
7. worker/*.go Register()    — mux.HandleFunc(TypeX, ...)
8. locales                  — errors.*, <platform>.* keys (4 files — see locale-sync skill)
9. runtime/registry.go      — admin-tunable settings (if any)
10. config/config.go        — new env fields + .env.example
```

## dispatchLink pattern

Reference: `go/internal/handler/bot.go` → `dispatchLink()`.

Typical branches:
- **Music card** (Spotify/SoundCloud) → `handler/music.go`, `MusicPayload`
- **YouTube session** → `handler/youtube.go`, quality/ratio callbacks
- **Direct enqueue** → `enqueueOrReplyError()` with `queue.TypeDownload` or platform-specific type

## Queue task types

Defined in `go/internal/queue/client.go`:

| Constant | Value |
|----------|-------|
| `TypeDownload` | `download:send` |
| `TypeTikTok` | `download:tiktok` |
| `TypePinterest` | `download:pinterest` |
| `TypeSpotify` | `download:spotify` |
| `TypeSoundCloud` | `download:soundcloud` |
| `TypeTikTokCarousel` | `download:tiktok_carousel` |
| `TypeBroadcast` | `broadcast:execute` |

Download tasks use `MaxRetry(0)` — no automatic retry.

## Payload structs

- `DownloadPayload` — URL downloads (YouTube, TikTok, X, Instagram, Pinterest)
- `MusicPayload` — Spotify/SoundCloud with `ReleaseJSON`
- `BroadcastPayload` — admin broadcasts

Always set `LockToken`, `LockScene`, `ChatID`, `UserID`, `Lang` when enqueueing.

## User lock

Before enqueue: acquire via `redisx` user lock (`user_busy:{userID}`).
Release in worker defer or on cancel (`handler/cancel.go`, callbacks `dlc:` / `dlq:`).

## Callback prefix conventions

Registered in `go/internal/handler/bot.go`:

| Prefix | Purpose |
|--------|---------|
| `lang\|` | Language picker |
| `quality:` / `ratio:` | YouTube session |
| `settings\|` | User settings |
| `dlc:` | Cancel active download |
| `dlq:` | Download queue UI |
| `admin\|` | Admin panel FSM |
| `broadcast\|` | Broadcast wizard |
| `ttk:img:` | TikTok carousel photos |

**Telegram limit:** callback_data max **64 bytes** — keep tokens short.

## Reference implementations

| Complexity | Files |
|------------|-------|
| Simple enqueue | Pinterest/TikTok path in `handler/bot.go` |
| Session + callbacks | `handler/youtube.go`, `youtube/session.go` |
| Music cards | `handler/music.go`, `spotify/`, `soundcloud/` |
| Platform worker | `worker/pinterest_tiktok.go`, `worker/music.go` |
| yt-dlp subprocess | `ytdlp/downloader.go`, `worker/ytdlp_opts.go` |

## After implementation

```bash
cd go && go build ./... && go test ./...
scripts/check-parity.sh
```

Update Python parity if the feature exists in both stacks (see saveinator-python-go-parity).

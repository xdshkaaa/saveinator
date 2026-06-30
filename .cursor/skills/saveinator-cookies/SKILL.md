---
name: saveinator-cookies
description: >-
  Exports and deploys TikTok/Instagram cookies for Saveinator. Use when TikTok cookies,
  Instagram cookies, cookie refresh, login required, private content, or yt-dlp cookies.
---

# Saveinator Cookies

TikTok and Instagram downloads may require browser cookies. Cookies flow: **export locally → scp to VPS → mount read-only → sync to writable temp in container**.

## Export (local)

```bash
scripts/export_tiktok_cookies.sh [output_path]      # default: secrets/tiktok_cookies.txt
scripts/export_instagram_cookies.sh [output_path]   # default: secrets/instagram_cookies.txt
```

Uses `yt-dlp --cookies-from-browser` with **probe URLs** (not homepages):

| Platform | Default probe |
|----------|---------------|
| TikTok | `https://www.tiktok.com/@tiktok/video/7106594319421886977` |
| Instagram | `https://www.instagram.com/reel/DaAl-AKqLRF/` |

Override browser: `TIKTOK_COOKIES_FROM_BROWSER`, `INSTAGRAM_COOKIES_FROM_BROWSER` (default `chrome`).

### Gotchas

- **TikTok:** probe URL triggers extraction; homepage URLs fail as "Unsupported URL"
- **Instagram:** do **not** pre-create empty cookie file — yt-dlp treats `--cookies` as input and fails
- **Instagram:** verify `sessionid` in exported file; macOS may need Full Disk Access for Terminal/Cursor
- Optional: use `.venv/bin/yt-dlp` if present locally
- **Never commit** `secrets/*.txt` cookie files

## Deploy to VPS

```bash
scripts/deploy_tiktok_cookies.sh
scripts/deploy_instagram_cookies.sh
```

Flow:
1. Export locally
2. `scp` → `/opt/saveinator/secrets/`
3. Patch `.env`: `TIKTOK_COOKIES_PATH=/secrets/tiktok_cookies.txt` (or Instagram equivalent)
4. `docker compose up -d --force-recreate saveinator`

Container mounts `/secrets` read-only.

## In-container sync

`go/internal/cookies/sync.go` copies mount → writable temp when mount is newer:

| Platform | Mount path (env) | Writable path |
|----------|------------------|---------------|
| TikTok | `TIKTOK_COOKIES_PATH` | `/tmp/tiktok_cookies.txt` |
| Instagram | `INSTAGRAM_COOKIES_PATH` | `/tmp/instagram_cookies.txt` |

Pinterest optional: `PINTEREST_COOKIES_PATH` (passed to Pinterest API client).

## Auto-refresh (in-process)

`go/internal/worker/maintenance.go` every **5 minutes**:
- `refreshTikTokCookies` — uses `TIKTOK_COOKIES_REFRESH_URL`, `TIKTOK_COOKIES_REFRESH_ENABLED`
- `refreshInstagramCookies` — uses `INSTAGRAM_COOKIES_REFRESH_URL`, `INSTAGRAM_COOKIES_REFRESH_ENABLED`

Requires cookies path or `*_COOKIES_FROM_BROWSER` configured.

## Env vars (see `.env.example`)

| Variable | Purpose |
|----------|---------|
| `TIKTOK_COOKIES_PATH` | Netscape cookie file in container |
| `TIKTOK_COOKIES_FROM_BROWSER` | Browser name for yt-dlp refresh |
| `TIKTOK_COOKIES_REFRESH_ENABLED` | Enable auto-refresh |
| `TIKTOK_COOKIES_REFRESH_URL` | Probe URL for refresh |
| `INSTAGRAM_COOKIES_PATH` | Same pattern for Instagram |
| `INSTAGRAM_COOKIES_FROM_BROWSER` | Browser for Instagram refresh |
| `INSTAGRAM_COOKIES_REFRESH_ENABLED` | Enable auto-refresh |
| `INSTAGRAM_COOKIES_REFRESH_URL` | Probe URL for refresh |
| `PINTEREST_COOKIES_PATH` | Optional private Pinterest pins |

## Troubleshooting

| Problem | Fix |
|---------|-----|
| Empty export | Log into platform in browser; check probe URL |
| Instagram no `sessionid` | Re-login; grant Full Disk Access on macOS |
| Cookies not picked up | Force-recreate `saveinator` after deploy |
| Stale cookies | Re-run deploy scripts or enable refresh URLs |

---
name: saveinator-cookies
description: >-
  Exports and deploys TikTok cookies for Saveinator. Use when TikTok cookies,
  cookie refresh, login required, private content, or yt-dlp cookies.
---

# Saveinator Cookies

TikTok downloads may require browser cookies. Cookies flow: **export locally → scp to VPS → mount read-only → sync to writable temp in container**.

## Export (local)

```bash
scripts/export_tiktok_cookies.sh [output_path]      # default: secrets/tiktok_cookies.txt
```

Uses `yt-dlp --cookies-from-browser` with **probe URLs** (not homepages):

| Platform | Default probe |
|----------|---------------|
| TikTok | `https://www.tiktok.com/@tiktok/video/7106594319421886977` |

Override browser: `TIKTOK_COOKIES_FROM_BROWSER` (default `chrome`).

### Gotchas

- **TikTok:** probe URL triggers extraction; homepage URLs fail as "Unsupported URL"
- Optional: use `.venv/bin/yt-dlp` if present locally
- **Never commit** `secrets/*.txt` cookie files

## Deploy to VPS

```bash
scripts/deploy_tiktok_cookies.sh
```

Flow:
1. Export locally
2. `scp` → `/opt/saveinator/secrets/`
3. Patch `.env`: `TIKTOK_COOKIES_PATH=/secrets/tiktok_cookies.txt`
4. `docker compose up -d --force-recreate saveinator`

Container mounts `/secrets` read-only.

## In-container sync

`go/internal/cookies/sync.go` copies mount → writable temp when mount is newer:

| Platform | Mount path (env) | Writable path |
|----------|------------------|---------------|
| TikTok | `TIKTOK_COOKIES_PATH` | `/tmp/tiktok_cookies.txt` |

Pinterest optional: `PINTEREST_COOKIES_PATH` (passed to Pinterest API client).

## Auto-refresh (in-process)

`go/internal/worker/maintenance.go` every **5 minutes**:
- `refreshTikTokCookies` — uses `TIKTOK_COOKIES_REFRESH_URL`, `TIKTOK_COOKIES_REFRESH_ENABLED`

Requires cookies path or `TIKTOK_COOKIES_FROM_BROWSER` configured.

## Env vars (see `.env.example`)

| Variable | Purpose |
|----------|---------|
| `TIKTOK_COOKIES_PATH` | Netscape cookie file in container |
| `TIKTOK_COOKIES_FROM_BROWSER` | Browser name for yt-dlp refresh |
| `TIKTOK_COOKIES_REFRESH_ENABLED` | Enable auto-refresh |
| `TIKTOK_COOKIES_REFRESH_URL` | Probe URL for refresh |
| `PINTEREST_COOKIES_PATH` | Optional private Pinterest pins |

## Troubleshooting

| Problem | Fix |
|---------|-----|
| Empty export | Log into platform in browser; check probe URL |
| Cookies not picked up | Force-recreate `saveinator` after deploy |
| Stale cookies | Re-run deploy scripts or enable refresh URLs |

# Product

<!-- impeccable:product-schema 1 -->

## Platform

web

## Users

One operator — the owner, technically fluent. Uses the dashboard daily to check the health of the Saveinator Telegram bot fleet, watch activity, downloads, and users, and spot anomalies quickly.

## Product Purpose

A single-pane operational status view over the Saveinator service: Telegram bots that download media from YouTube, TikTok, X/Twitter, Spotify, SoundCloud, and Pinterest. Success is understanding "is everything working, and what is happening right now" at a glance.

## Positioning

An operator's control surface for a multi-bot media-downlactor service: services health, key metrics, 14-day download timeline, per-platform breakdown, per-bot stats, and a per-user download history drill-down — one screen, real production data, no marketing layer.

## Operating Context

- Served at dash-saveinator.xdshka.party through Cloudflare Tunnel → Caddy :8098 with basic auth.
- Views the page from desktop and mobile browsers; page auto-refreshes every 30 seconds.
- All copy is Russian; the operator habitually reads it in short glances and reacts to anomalies (service down, failed downloads, inactive users).
- Deployed via `scripts/deploy-dash.sh`: git push to `main` → docker compose build/up `dash` on the VPS (45.128.235.219) → patch Caddy + cloudflared → verify with curl health/auth/public checks.

## Capabilities and Constraints

Confirmed dashboard sections (all must survive redesign):
- Services strip (name, up/down, latency ms).
- Six KPIs: total users, new today (delta vs yesterday), downloads today (delta 7d), active now (30-min window), DAU/MAU + stickiness, 30-day success rate + failed count.
- 14-day downloads timeline (canvas; total/completed/failed).
- 30-day platform breakdown (downloads + success rate per platform).
- Bots grid (users, downloads, 30d downloads, 30d failures).
- Users table (id, handle/name, language, bot, registration, downloads, errors, activity; search + sort, limit 200) with row click → per-user download history drawer (200 items; status, platform, time, size, bot, url, error).
- Test URLs checklist («Тест ссылок»): the admin adds links (youtube, tiktok, instagram, x, pinterest only); the saveinator worker runs each through its full production scenario into a throwaway temp dir (nothing is sent to Telegram, `downloads` stats untouched) and reports PASSED/FAILED with media size, duration and error text; per-row rerun/delete, «Прогнать все» requeues every finished link, new links are queued automatically; header shows passed/failed/waiting counts; checklist poll every 5 s.

API surface: `/api/overview`, `/api/downloads?days=14`, `/api/platforms?days=30`, `/api/users?sort&limit`, `/api/users/{id}/downloads?limit`, `/api/services`, `/api/test-urls` (GET list+counts, POST add), `/api/test-urls/run` (POST requeue all), `/api/test-urls/{id}/rerun` (POST), `/api/test-urls/{id}` (DELETE), `/api/health`.

The overview payload also carries fields the current UI does not render: `wau`, `users_with_downloads`, `returning_users`, `languages`, `new_7d`, `new_30d`, `banned`.

Technical constraints: static files are embedded into the Go binary (`//go:embed static/*`); no build step beyond Go; keep the app a single static HTML/CSS/JS page served at `/`. Auto-refresh 30s and basic auth are fixed. Dark or light theme is free to choose.

## Brand Commitments

- Product name "SAVEINATOR · DASH" and the domain dash-saveinator.xdshka.party.
- Russian interface copy throughout.
- All existing sections, data, and behavior are preserved; the redesign replaces the visual world only.
- Refesh rhythm (30 s), access model (Caddy basic auth behind Cloudflare Tunnel), and deploy path (deploy-dash.sh) are fixed.

## Evidence on Hand

Real production data is available via the live API on the VPS (45.128.235.219). No invented metrics, testimonials, or claims. Status labels (в очереди, получение, скачивание, конвертация, отправка, отправлено, ошибка) and platform names (YouTube, TikTok, Instagram, X, Spotify, SoundCloud, Pinterest, Yandex Music) are established.

## Product Principles

1. Glance-first: an anomaly must be visible within seconds, from across a room.
2. The data is the hero: real numbers, honest states, no decorative proxy for information.
3. Preserve behavior: every section, the 30s rhythm, auth, and the deploy path stay intact.
4. Built for one technical operator: density is acceptable, clarity is not optional.
5. Trust over flash: the interface earns belief by being precise and stable, not clever.

## Accessibility & Inclusion

No product-specific audience requirement beyond existing care: keyboard-operable users table, visible focus states, and `prefers-reduced-motion` support are already present and must be preserved.
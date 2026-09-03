# Saveinator Monitoring

Human ops reference. For agent workflows see `.cursor/skills/saveinator-monitoring/SKILL.md`.

Production monitoring on VPS (`YOUR_VPS_IP`).

## Quick start

```bash
cd /opt/saveinator
docker compose up -d   # app stack first (creates saveinator_default network)
cp .env.monitoring.example .env.monitoring
docker compose -f docker-compose.monitoring.yml --env-file .env.monitoring up -d
```

**Slim mode** (low memory): stop heavy services, then `docker compose -f docker-compose.monitoring.slim.yml --env-file .env.monitoring up -d`.

## Public URLs

| URL | Purpose |
|-----|---------|
| https://saveinator.xdshka.party | Grafana |
| https://saveinator-hooks.xdshka.party/webhook | Telegram webhook |
| https://dash-saveinator.xdshka.party | Operator dashboard (Basic auth) |

## Operator dashboard (`dash`)

Standalone Go service (`go/cmd/dash`), read-only consumer of Postgres/Redis; serves a static UI + JSON API on `127.0.0.1:9000` (container `dash` in `docker-compose.yml`). Shows per-service status (probes from `DASH_SERVICE_PROBES`), aggregate stats, per-platform/per-bot breakdown and the full user table.

Exposed via Caddy `:8098` (block in `monitoring/caddy-grafana.caddyfile`) with Basic auth — bcrypt hash in `/etc/caddy/dash.htpasswd` on the VPS (`DASH_AUTH_USER`/`DASH_AUTH_PASSWORD` env), then Cloudflare Tunnel (`dash-saveinator.xdshka.party` → `http://localhost:8098`).

Deploy: `DASH_AUTH_USER=... DASH_AUTH_PASSWORD=... VPS_HOST=45.128.235.219 ./scripts/deploy-dash.sh` — builds the container, patches (not replaces) the shared `/etc/caddy/Caddyfile` and `/etc/cloudflared/config.yml` with backups, reloads both proxies and verifies 401/200.

Verify:

```bash
curl -fsS http://127.0.0.1:9000/api/health
curl -s -o /dev/null -w '%{http_code}\n' http://127.0.0.1:8098/            # 401
curl -s -o /dev/null -w '%{http_code}\n' -u USER:PASS http://127.0.0.1:8098/  # 200
```

## Verify

```bash
curl -s http://127.0.0.1:9091/api/v1/targets | python3 -m json.tool | grep health
curl -s http://127.0.0.1:9101/metrics | head
curl -s http://127.0.0.1:9102/metrics | head
```

## One-time setup

1. Caddy block: `monitoring/caddy-grafana.caddyfile` → `/etc/caddy/Caddyfile`
2. Cloudflared: `monitoring/cloudflared-ingress.yml` before final `http_status:404`
3. DNS: both `saveinator.xdshka.party` and `saveinator-hooks.xdshka.party` → tunnel

## Dashboards

Auto-provisioned from `monitoring/grafana/dashboards/`. Manual import: Grafana → Import → upload JSON.

`bot-fleet.json` is the primary dashboard for the botd fleet (pinterest, pinterest_kz, soundcloud, spotify): per-bot `saveinator_bot_updates_total`, `saveinator_bot_downloads_enqueued_total`, `saveinator_bot_tasks_total{status}`, `saveinator_bot_task_duration_seconds` — all scraped from `:9106` (job `saveinator-botd`).

`worker-celery.json` uses `saveinator_celery_tasks_total` (asynq worker inside Go `saveinator`).

## Security

- Prometheus not exposed publicly
- Webhook host proxies `/`, `/health`, `/webhook*` only; metrics stay localhost
- Change `GRAFANA_ADMIN_PASSWORD` after first login
- Do not commit `.env.monitoring`

## Stop

```bash
docker compose -f docker-compose.monitoring.yml --env-file .env.monitoring down
```

App stack is unaffected.

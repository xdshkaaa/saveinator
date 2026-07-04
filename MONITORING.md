# Saveinator Monitoring

Human ops reference. For agent workflows see `.cursor/skills/saveinator-monitoring/SKILL.md`.

Production monitoring on VPS (`103.214.69.38`).

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

---
name: saveinator-monitoring
description: >-
  Runs Saveinator monitoring stack on VPS — Prometheus, Grafana, alerts, Loki.
  Use when setting up Grafana, Prometheus, monitoring, alerts, dashboards, Loki,
  or saveinator.xdshka.party.
---

# Saveinator Monitoring

Production monitoring on VPS (`103.214.69.38`). See also [`MONITORING.md`](MONITORING.md) for human ops reference.

## Prerequisites

App stack must run first (creates `saveinator_default` Docker network):

```bash
cd /opt/saveinator && docker compose up -d
```

## Quick start (full stack)

```bash
cd /opt/saveinator
cp .env.monitoring.example .env.monitoring
# Edit GRAFANA_ADMIN_PASSWORD and DB_PASSWORD
docker compose -f docker-compose.monitoring.yml --env-file .env.monitoring up -d
```

## Slim mode (low memory VPS)

Keeps Grafana, Prometheus, VPS metrics, bot/worker metrics only:

```bash
cd /opt/saveinator
docker compose -f docker-compose.monitoring.yml --env-file .env.monitoring stop \
  alertmanager cadvisor postgres_exporter redis_exporter loki promtail
docker compose -f docker-compose.monitoring.slim.yml --env-file .env.monitoring up -d
```

## Services (localhost)

| Service | Port | Purpose |
|---------|------|---------|
| Prometheus | `9091` | Metrics + alerting |
| Grafana | `3000` | Dashboards |
| Alertmanager | `9093` | Alert routing |
| node_exporter | `9100` | VPS CPU/RAM/disk |
| cAdvisor | `8180` | Docker containers |
| Bot metrics | `9101` | App `/metrics` |
| Worker metrics | `9102` | Same registry, worker-compat port |
| postgres_exporter | `9187` | PostgreSQL |
| redis_exporter | `9121` | Redis |
| Loki | `3100` | Log aggregation |
| Promtail | `9080` | Docker log shipping |

## Public URLs

| URL | Purpose |
|-----|---------|
| `https://saveinator.xdshka.party` | Grafana (Caddy + Cloudflare Tunnel) |
| `https://saveinator-hooks.xdshka.party/webhook` | Telegram webhook (separate host) |

Grafana credentials: `.env.monitoring` → `GRAFANA_ADMIN_PASSWORD`.

## One-time origin routing

1. Add Caddy block from `monitoring/caddy-grafana.caddyfile` to `/etc/caddy/Caddyfile`
2. Add Cloudflared ingress from `monitoring/cloudflared-ingress.yml` **before** final `http_status:404`
3. DNS: point both `saveinator.xdshka.party` and `saveinator-hooks.xdshka.party` at tunnel

```bash
sudo caddy validate --config /etc/caddy/Caddyfile && sudo systemctl reload caddy
sudo systemctl restart cloudflared
curl -fsSI https://saveinator.xdshka.party/login
curl -fsS https://saveinator-hooks.xdshka.party/health
```

## Dashboards (auto-provisioned)

| Dashboard | File |
|-----------|------|
| VPS Overview | `monitoring/grafana/dashboards/vps-overview.json` |
| Docker Containers | `monitoring/grafana/dashboards/docker-containers.json` |
| Telegram Bots | `monitoring/grafana/dashboards/telegram-bots.json` |
| Saveinator Operations | `monitoring/grafana/dashboards/operations.json` |
| Download Operations | `monitoring/grafana/dashboards/downloads.json` |
| Worker / Celery | `monitoring/grafana/dashboards/worker-celery.json` |
| User Activity | `monitoring/grafana/dashboards/user-activity.json` |
| Error & Reliability | `monitoring/grafana/dashboards/reliability-errors.json` |
| PostgreSQL & Redis | `monitoring/grafana/dashboards/data-stores.json` |
| Logs | `monitoring/grafana/dashboards/logs.json` |

**Note:** `worker-celery.json` uses metric `saveinator_celery_tasks_total` — kept for dashboard compatibility; values come from asynq worker inside Go `saveinator`.

## Key app metrics

| Metric | Description |
|--------|-------------|
| `saveinator_downloads_enqueued_total` | Jobs by platform |
| `saveinator_celery_tasks_total` | Worker task counts (asynq alias) |
| `saveinator_ytdlp_errors_total` | Download failures |
| `saveinator_errors_total` | Errors by source |

Disable: `METRICS_ENABLED=false` in app `.env`.

## Troubleshooting

| Problem | Fix |
|---------|-----|
| `saveinator_default` network not found | Start app stack: `docker compose up -d` |
| Bot target DOWN | `curl http://127.0.0.1:9101/metrics`; check `METRICS_ENABLED=true` |
| postgres_exporter DOWN | `DB_PASSWORD` in `.env.monitoring` must match app `.env` |
| No logs in Loki | `docker logs saveinator-promtail`; check Docker socket mount |

## Add another bot

1. Expose `/metrics` on unique localhost port (e.g. `9103`)
2. Add scrape job in `monitoring/prometheus/prometheus.yml`
3. `curl -X POST http://127.0.0.1:9091/-/reload`

## Stop monitoring

```bash
docker compose -f docker-compose.monitoring.yml --env-file .env.monitoring down
```

App stack is unaffected.

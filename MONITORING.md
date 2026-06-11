# Saveinator Monitoring

Production monitoring stack for the Saveinator bot VPS (`31.76.76.12`).

## What runs

| Service | Port (localhost) | Purpose |
|---------|------------------|---------|
| Prometheus | `9090` | Metrics storage and alerting |
| Grafana | `3000` | Dashboards |
| Alertmanager | `9093` | Alert routing |
| node_exporter | `9100` | VPS CPU/RAM/disk/network |
| cAdvisor | `8080` | Docker container metrics |
| Bot metrics | `9101` | Telegram bot `/metrics` |
| Worker metrics | `9102` | Celery worker `/metrics` |
| postgres_exporter | `9187` | PostgreSQL metrics |
| redis_exporter | `9121` | Redis metrics |
| Loki | `3100` | Log aggregation |
| Promtail | `9080` | Docker log shipping |

All services bind to `127.0.0.1` except exporters that use host networking internally.

## Prerequisites

1. App stack must be running first:

```bash
cd /opt/saveinator
docker compose up -d
```

2. Docker network `saveinator_default` must exist (created automatically by app compose).

## Quick start

```bash
cd /opt/saveinator
cp .env.monitoring.example .env.monitoring
# Edit GRAFANA_ADMIN_PASSWORD and DB_PASSWORD
docker compose -f docker-compose.monitoring.yml --env-file .env.monitoring up -d
```

## Verify

```bash
# Prometheus targets (all should be UP)
curl -s http://127.0.0.1:9090/api/v1/targets | python3 -m json.tool | grep health

# Bot metrics
curl -s http://127.0.0.1:9101/metrics | head

# Worker metrics
curl -s http://127.0.0.1:9102/metrics | head
```

## Grafana access

Grafana listens on `127.0.0.1:3000` only. Use SSH tunnel from your laptop:

```bash
ssh -L 3000:127.0.0.1:3000 root@31.76.76.12
```

Open http://localhost:3000 and log in with credentials from `.env.monitoring`.

## Dashboards (auto-provisioned)

| Dashboard | File |
|-----------|------|
| VPS Overview | `monitoring/grafana/dashboards/vps-overview.json` |
| Docker Containers | `monitoring/grafana/dashboards/docker-containers.json` |
| Telegram Bots | `monitoring/grafana/dashboards/telegram-bots.json` |
| Logs | `monitoring/grafana/dashboards/logs.json` |

Manual re-import: Grafana → Dashboards → Import → upload JSON file.

## Security

- Prometheus is **not** exposed publicly.
- Change `GRAFANA_ADMIN_PASSWORD` immediately after first login.
- Do not commit `.env.monitoring` or bot tokens.
- Optional: put Grafana behind Caddy/nginx with basic auth for remote access.

## Bot metrics exposed

| Metric | Description |
|--------|-------------|
| `saveinator_uptime_seconds` | Bot process uptime |
| `saveinator_messages_received_total` | Messages received |
| `saveinator_commands_handled_total` | Commands handled |
| `saveinator_errors_total` | Errors by source |
| `saveinator_telegram_api_*` | Telegram API requests, latency, failures |
| `saveinator_downloads_enqueued_total` | Download jobs started (`platform`: youtube, tiktok, instagram, x, pinterest, spotify) |
| `saveinator_celery_tasks_total` | Worker task counts |
| `saveinator_ytdlp_errors_total` | Download failures |

Disable metrics: `METRICS_ENABLED=false` in app `.env`.

## Add another bot to monitoring

1. Expose `/metrics` on a unique localhost port (e.g. `9103`).
2. Add scrape job in `monitoring/prometheus/prometheus.yml`:

```yaml
  - job_name: my-other-bot
    static_configs:
      - targets: ["127.0.0.1:9103"]
```

3. Reload Prometheus: `curl -X POST http://127.0.0.1:9090/-/reload`
4. Duplicate panels in `telegram-bots.json` or create a new dashboard JSON.

## Alerts

Rules live in `monitoring/prometheus/alerts.yml`:

- High CPU / RAM / low disk
- Bot/worker target down
- Error rate spike
- Telegram API failures
- No messages for 30 minutes
- Postgres / Redis down

Configure Alertmanager webhook in `monitoring/alertmanager/alertmanager.yml` if needed.

## Troubleshooting

| Problem | Fix |
|---------|-----|
| `saveinator_default` network not found | Start app stack first: `docker compose up -d` |
| Bot target DOWN | Check `curl http://127.0.0.1:9101/metrics` and `METRICS_ENABLED=true` |
| postgres_exporter DOWN | Verify `DB_PASSWORD` in `.env.monitoring` matches app `.env` |
| No logs in Loki | Check `docker logs saveinator-promtail` and Docker socket mount |

## Stop monitoring

```bash
docker compose -f docker-compose.monitoring.yml --env-file .env.monitoring down
```

App stack is unaffected.

# Saveinator Monitoring

Production monitoring stack for the Saveinator bot VPS (`103.214.69.38`).

## What runs

| Service | Port (localhost) | Purpose |
|---------|------------------|---------|
| Prometheus | `9091` | Metrics storage and alerting |
| Grafana | `3000` | Dashboards |
| Alertmanager | `9093` | Alert routing |
| node_exporter | `9100` | VPS CPU/RAM/disk/network |
| cAdvisor | `8180` | Docker container metrics |
| Bot metrics | `9101` | Telegram bot `/metrics` |
| Bot webhook | `8000` | Telegram webhook HTTP app (localhost only) |
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
curl -s http://127.0.0.1:9091/api/v1/targets | python3 -m json.tool | grep health

# Bot metrics
curl -s http://127.0.0.1:9101/metrics | head

# Worker metrics
curl -s http://127.0.0.1:9102/metrics | head
```

## Grafana access

Grafana listens on `127.0.0.1:3000` and is exposed through Caddy + Cloudflare Tunnel at:

**https://saveinator.xdshka.party**

Telegram webhook traffic is exposed separately at:

**https://saveinator-hooks.xdshka.party/webhook**

Log in to Grafana with credentials from `.env.monitoring`.

### Origin routing (once per VPS)

1. Add the Caddy block from `monitoring/caddy-grafana.caddyfile` to `/etc/caddy/Caddyfile`, then:

```bash
sudo caddy validate --config /etc/caddy/Caddyfile
sudo systemctl reload caddy
```

2. Add the Cloudflared ingress rule from `monitoring/cloudflared-ingress.yml` before the final `http_status:404` rule, then:

```bash
sudo systemctl restart cloudflared
```

3. In Cloudflare DNS, point both `saveinator.xdshka.party` and `saveinator-hooks.xdshka.party` at the tunnel (same CNAME pattern as other `*.xdshka.party` hosts).

Verify:

```bash
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

Manual re-import: Grafana → Dashboards → Import → upload JSON file.

## Security

- Prometheus is **not** exposed publicly.
- The public webhook host only proxies `/`, `/health`, and `/webhook*`; bot `/metrics` stays localhost-only.
- Change `GRAFANA_ADMIN_PASSWORD` immediately after first login.
- Do not commit `.env.monitoring` or bot tokens.
- Optional: put Grafana behind Caddy with Grafana login (already configured for `saveinator.xdshka.party`).

## Bot metrics exposed

| Metric | Description |
|--------|-------------|
| `saveinator_uptime_seconds` | Bot process uptime |
| `saveinator_messages_received_total` | Messages received |
| `saveinator_commands_handled_total` | Commands handled |
| `saveinator_errors_total` | Errors by source |
| `saveinator_telegram_api_*` | Telegram API requests, latency, failures |
| `saveinator_http_*` | Internal HTTP/API route request counts and latency |
| `saveinator_users_created_total` | Users created in the database |
| `saveinator_downloads_enqueued_total` | Download jobs started (`platform`: youtube, tiktok, instagram, x, pinterest, spotify, soundcloud) |
| `saveinator_download_file_size_bytes` | Completed download file size distribution |
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

3. Reload Prometheus: `curl -X POST http://127.0.0.1:9091/-/reload`
4. Duplicate panels in `telegram-bots.json` or create a new dashboard JSON.

## Alerts

Rules live in `monitoring/prometheus/alerts.yml`:

- High CPU / RAM / low disk
- Bot/worker target down
- Error rate spike
- Telegram API failures
- No messages for 30 minutes
- High download failure rate / timeout-like failures
- Queue backlog too high
- Worker task duration too high
- Postgres / Redis down
- Loki no logs received
- Prometheus target down

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

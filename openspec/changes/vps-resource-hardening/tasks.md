## 1. Internal API auth (`/download/pinterest`)

- [x] 1.1 Add `INTERNAL_API_TOKEN` to `go/internal/config/config.go` (required, no default — fail fast if unset while `DOWNLOAD_API_ENABLED=true`)
- [x] 1.2 Add constant-time `X-Internal-Token` check at the top of `PinterestHandler.ServeHTTP` in `go/internal/api/pinterest.go`, returning `401` before body is read
- [x] 1.3 Add per-IP rate limiting to the same handler using `internal/redisx.AllowRateLimit`, returning `429` when exceeded
- [x] 1.4 Add/update tests in `go/internal/api/pinterest_test.go` covering: missing token → 401, wrong token → 401, valid token → existing behavior, over rate limit → 429
- [x] 1.5 Add `INTERNAL_API_TOKEN` to `.env.example` and `docker-compose.yml` (both `saveinator` and `botd` service environments, plus wherever `DOWNLOAD_API_ENABLED` is read)

## 2. Worker concurrency

- [x] 2.1 Change `asynq.Config.Concurrency` from `2` to `1` in `go/internal/app/app.go` (also applied to `internal/botkit/run.go` and the `internal/botkit/fleet.go` per-bot default/fallback, and `bots.yaml` per-bot `concurrency: 2` → `1` for all 4 fleet bots — these were additional concurrency sites not called out in the design, found during implementation; without this the botd fleet alone could run up to 8 concurrent workers on 1 core)
- [x] 2.2 Confirmed: `saveinator` uses `internal/app/app.go` (mode `all`); `botd` fleet uses `internal/botkit/fleet.go`; standalone single-bot binaries (pinterest_kz etc.) use `internal/botkit/run.go` — all three sites now set `Concurrency: 1`
- [x] 2.3 Checked `CLAUDE.md` — concurrency not mentioned as a tunable, nothing to update

## 3. Monitoring stack trim

- [x] 3.1 Remove `cadvisor`, `loki`, `promtail` services from `docker-compose.monitoring.yml`
- [x] 3.2 Remove now-unused `loki-data` and `promtail-data` volumes from the same file
- [x] 3.3 Remove the `cadvisor` scrape job (and loki/promtail jobs) from `monitoring/prometheus/prometheus.yml`
- [x] 3.4 Add `mem_limit` to `prometheus`, `alertmanager`, `grafana`, `node_exporter`, `postgres_exporter`, `redis_exporter` in `docker-compose.monitoring.yml`
- [x] 3.5 Change Prometheus `--storage.tsdb.retention.time` from `30d` to `14d`
- [x] 3.6 Removed `dashboards/docker-containers.json` (100% cAdvisor-sourced) and `dashboards/logs.json` (100% Loki-sourced); removed the `Loki` datasource from `provisioning/datasources/prometheus.yml`. **Follow-up not done**: `reliability-errors.json`, `downloads.json`, `operations.json`, `user-activity.json`, `pinterest-bot.json`, `data-stores.json` each embed 1-3 individual Loki-datasource log panels that will now show "datasource not found" in Grafana — those dashboards are otherwise Prometheus-based and were kept intact rather than risk broad JSON surgery; `worker-celery.json` has one cAdvisor-metric panel that will render empty (not an error, just no data) since it queries Prometheus, not Loki directly.

## 4. Rollout

- [ ] 4.1 On the VPS, check Caddy/access logs for any historical traffic to `/download/pinterest` before enabling auth
- [ ] 4.2 Set `INTERNAL_API_TOKEN` in the VPS `.env` before deploying code
- [ ] 4.3 Deploy app changes via `scripts/deploy.sh`; verify `curl -X POST /download/pinterest` without header returns 401, with correct header behaves as before
- [ ] 4.4 Deploy monitoring changes via `docker compose -f docker-compose.monitoring.yml up -d`; verify `docker ps` no longer shows cadvisor/loki/promtail and remaining containers are healthy
- [ ] 4.5 Watch `docker stats` and asynq/Prometheus queue metrics for at least one busy period to confirm no regression from `Concurrency: 1`

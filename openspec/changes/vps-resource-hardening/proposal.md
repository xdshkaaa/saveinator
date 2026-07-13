## Why

saveinator runs on a 4GB RAM / 1 vCPU VPS but the current deployment stacks 13 containers (app services + a 9-container monitoring stack) with no memory limits on most of them, runs up to 4 concurrent CPU-bound worker jobs against 1 core, and exposes an unauthenticated HTTP download endpoint on the public-facing port. An explore-mode audit (2026-07-13) found these as the highest-risk gaps for stability and abuse on this box size. Fixing them now avoids OOM kills during traffic spikes and closes an open DoS vector before it's found.

## What Changes

- Drop `cadvisor` and `loki`+`promtail` from `docker-compose.monitoring.yml` (heaviest, least essential for a solo-op box); keep prometheus/alertmanager/grafana/node_exporter/postgres_exporter/redis_exporter.
- Add `mem_limit` to every remaining monitoring container.
- Reduce Prometheus `--storage.tsdb.retention.time` from 30d to 14d.
- Add authentication to `POST /download/pinterest` (`go/internal/api/pinterest.go`): require a shared-secret `X-Internal-Token` header, reject unauthenticated requests with 401. **BREAKING** for any existing caller of this endpoint — no caller exists inside this repo (the Telegram Pinterest flow uses its own `internal/pinterest` client directly, not this HTTP API), so this is believed to be an external/unused surface, but any external consumer must be given the token out of band before rollout.
- Add per-IP rate limiting to `/download/pinterest` reusing the existing `internal/redisx.AllowRateLimit` pattern.
- Reduce asynq worker concurrency to fit 1 vCPU: change `Concurrency: 2` → `Concurrency: 1` per process in `go/internal/app/app.go` (applies to both `saveinator` and `botd`, since both run in mode `all`).

## Capabilities

### New Capabilities
- `internal-api-auth`: shared-secret authentication and per-IP rate limiting for internal-only HTTP endpoints (starting with `/download/pinterest`).
- `worker-resource-budget`: explicit, documented concurrency/resource ceilings for asynq workers sized to the deployment's CPU budget.

### Modified Capabilities
(none — no existing specs in this repo yet)

## Impact

- **Code**: `go/internal/api/pinterest.go` (auth + rate limit), `go/internal/app/app.go` (asynq `Concurrency`).
- **Config**: new env var `INTERNAL_API_TOKEN` (required on both `saveinator` and `botd` services in `docker-compose.yml`); `docker-compose.monitoring.yml` loses 3 services, gains `mem_limit` on the rest; `monitoring/prometheus/prometheus.yml` retention flag change (in `docker-compose.monitoring.yml` command args).
- **Ops**: one-time VPS deploy step to set `INTERNAL_API_TOKEN` before rollout; monitoring container removal means losing cAdvisor per-container dashboards and centralized log search in Grafana (Loki) — logs fall back to `docker logs`/journald.
- **Dependencies**: none added; three monitoring images removed (`gcr.io/cadvisor/cadvisor`, `grafana/loki`, `grafana/promtail`).

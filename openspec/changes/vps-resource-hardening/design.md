## Context

Deployment target: single VPS, 4GB RAM, 1 vCPU. Current stack (`docker-compose.yml` + `docker-compose.monitoring.yml`) runs 13 containers with no per-container memory limits on 9 of them (all of monitoring, plus `db`/`redis` rely only on internal soft limits: `shared_buffers=128MB` and `--maxmemory 128mb`). `saveinator` and `botd` are each capped at 768M via `deploy.resources.limits.memory`, and each runs asynq with `Concurrency: 2` in mode `all` (webhook + worker + metrics in one process), so up to 4 worker goroutines can attempt CPU-bound work (ffmpeg transcode) concurrently against 1 core.

`POST /download/pinterest` (`go/internal/api/pinterest.go:34`, registered via `RegisterDownloadRoutes` on the same `http.ServeMux` as the Telegram webhook in `go/internal/app/app.go:143`) has no authentication and no rate limiting. Grep across the repo found no in-repo caller — the Telegram-facing Pinterest flow uses `internal/pinterest.Client` directly, not this HTTP surface — so it is either an external integration point or dead code that's still reachable through whatever Caddy exposes on the VPS.

## Goals / Non-Goals

**Goals:**
- Keep the box from hitting OOM under normal + moderate burst load by cutting the largest unbounded memory consumers.
- Close the unauthenticated public HTTP surface without needing to know who (if anyone) currently calls it.
- Make worker concurrency match actual CPU budget (1 core) instead of an arbitrary default.

**Non-Goals:**
- Rearchitecting the monitoring stack (e.g. swapping Prometheus for something lighter) — this is a trim, not a replacement.
- Horizontal scaling / multi-core work distribution — out of scope while the box has 1 vCPU.
- General API hardening (CORS, request signing, mTLS) beyond a shared-secret header — proportionate to a single-tenant internal endpoint, not a public product API.
- Splitting I/O-bound vs CPU-bound work into separate semaphores (considered, see Decisions) — deferred to keep this change small and reversible.

## Decisions

**1. Drop cadvisor + loki + promtail rather than add limits to all 9 and keep them.**
cAdvisor runs `privileged: true`, mounts the full docker/cgroup tree, and scrapes it every 15s — the single heaviest and most invasive container in the stack, and its dashboards are a nice-to-have on a single-VPS deployment where `docker stats` covers the same need manually. Loki+promtail (log aggregation) add another two long-running processes plus a volume that grows unbounded; `docker logs` / journald already retain logs, so centralized search isn't worth the RAM on this box size. Kept: prometheus/alertmanager/grafana (dashboards + alerting are actively used per existing Grafana provisioning) and the three lightweight exporters (~15-30MB each).
*Alternative considered*: keep all 9, just add `mem_limit`. Rejected — caps prevent OOM-killing the host but don't reduce the steady-state footprint that's already tight on 4GB.

**2. Shared-secret header (`X-Internal-Token`) over mTLS or IP allowlisting.**
The endpoint's caller population is unknown (see Context), so IP allowlisting isn't reliably enforceable without breaking an unknown consumer, and mTLS is disproportionate operational overhead for one endpoint. A static bearer-style header checked with constant-time comparison is the minimum viable fix and matches the trust model (single shared secret between operator and whoever is authorized to call it).
*Alternative considered*: reuse `WEBHOOK_SECRET_TOKEN`. Rejected — that secret is Telegram's, scoping a second concern to it couples unrelated trust boundaries and complicates rotation.

**3. Per-IP rate limit via existing `internal/redisx.AllowRateLimit`, not a new limiter.**
The function already implements a scoped sliding-window limiter (used today for Telegram user/chat limits) and Redis is already a hard dependency of this service — reusing it avoids a new library or in-memory limiter that wouldn't work correctly with multiple processes (`saveinator` + `botd`) behind the same IP space.

**4. `Concurrency: 1` per process, not a split I/O/CPU semaphore.**
A split semaphore (yt-dlp fetch concurrent, ffmpeg transcode serialized) is the theoretically better throughput/latency trade-off, but it's a larger structural change (new synchronization primitive shared across download handlers) for a box that's already CPU-constrained enough that 1 concurrent heavy job at a time is the safe default. `Concurrency: 1` is a one-line change per process, immediately verifiable, and reversible. The split-semaphore approach is left as a documented follow-up if `Concurrency: 1` proves too slow under real traffic.
*Alternative considered*: split semaphore now. Rejected for this change — bigger surface area, harder to verify correctness under a time-boxed hardening pass; revisit if queue depth becomes a user-visible complaint after this ships.

## Risks / Trade-offs

- [Losing cAdvisor dashboards / Loki log search] → `docker stats` covers ad-hoc per-container checks; `docker logs -f <container>` / journald covers log debugging. Acceptable loss for a solo-operated box; can be reinstated later on a bigger VPS.
- [Unknown caller of `/download/pinterest` breaks silently after auth is added] → Impact is bounded (single endpoint, not the Telegram bot flow); mitigate by checking Caddy/access logs on the VPS before rollout to see if the path has real traffic, and communicating the new required header if any caller is found.
- [`Concurrency: 1` increases queue latency under concurrent user load] → Acceptable trade-off for a 1-core box where concurrent CPU-bound jobs don't actually parallelize; monitor via existing asynq/Prometheus metrics post-rollout, revisit with the split-semaphore design if latency complaints appear.
- [Reduced Prometheus retention (30d → 14d) loses longer-term trend data] → Acceptable for an ops-focused box; nothing in the repo depends on >14d retention today.

## Migration Plan

1. Set `INTERNAL_API_TOKEN` in the VPS `.env` before deploying the code change (avoids a window where the flag exists but the value is empty/unset).
2. Deploy code changes (`pinterest.go` auth + rate limit, `app.go` concurrency) via the standard `scripts/deploy.sh` flow.
3. Deploy `docker-compose.monitoring.yml` changes separately: `docker compose -f docker-compose.monitoring.yml up -d` after removing the three services (Compose will stop/remove containers no longer defined) and adding `mem_limit`.
4. Verify: `curl` `/download/pinterest` without the header returns 401; with the header, still functions; `docker stats` shows the three containers gone; asynq queue processes tasks with new concurrency.
5. Rollback: revert the compose file changes to bring cadvisor/loki/promtail back if log/metric visibility turns out to be needed; revert `Concurrency` and the auth check independently since they're unrelated code paths (no combined rollback needed).

## Open Questions

- Is `/download/pinterest` actually called by anything today? Needs a VPS-side check (Caddy access logs) before/during rollout — if traffic is found, its owner needs the new token communicated out of band.
- Is 14d Prometheus retention the right number, or does anyone rely on longer trend windows for capacity planning? Defaulting to 14d as a reasonable middle ground; adjustable without further design work.

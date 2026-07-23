## Context

`go/internal/xphotos/downloader.go` resolves tweet media by trying `api.fxtwitter.com` then `api.vxtwitter.com` in order (`fetchTweet`), each with its own JSON parser (`parseFxTwitter`, `parseVxTwitter`). Both are public, third-party, shared-rate-limit mirrors. FxEmbed (the maintained fork/successor of FxTwitter, https://docs.fxembed.com) can be self-hosted on Cloudflare Workers and serves the same `/status/:id` JSON shape as `api.fxtwitter.com` (FxEmbed is a drop-in API-compatible replacement). Deploying the Worker itself is out of scope for this change (operator does that separately per FxEmbed's deployment docs); this change only adds the Go client support for pointing at one once deployed.

## Goals / Non-Goals

**Goals:**
- Allow an operator to configure a self-hosted FxEmbed base URL via env var.
- When configured, try it first (lowest latency/most control), then fall back to the two existing public mirrors unchanged.
- Zero behavior change when the env var is unset (default/existing deployments unaffected).
- Reuse the existing FxTwitter-compatible JSON parser rather than writing a new one.

**Non-Goals:**
- Automating the actual Cloudflare Workers deployment of FxEmbed (git clone, wrangler, `npm run deploy`) — that's an operator/infra task per FxEmbed's own docs, not Go code.
- Changing video/gif resolution logic, `ytdlp` paths, or any other X-related download code outside `xphotos`.
- Adding retries/backoff beyond the existing single-attempt-per-source behavior.

## Decisions

- **Config surface**: add `FXEMBED_BASE_URL` (empty by default) to `go/internal/config/config.go`, threaded into `xphotos` via a package-level setter or a parameter on `DownloadMedia`/`FetchTweetMeta`, consistent with how other optional integrations are wired in this codebase (check `config.go` for the existing pattern before adding, e.g. a `Config` struct field vs. a package var — prefer whatever `xphotos` callers already use to reach it, likely `worker/download.go`).
  - Alternative considered: read `os.Getenv` directly inside `xphotos` — rejected because it bypasses the single `config` loading/normalization point and makes testing harder (can't inject a test URL without mutating process env).
- **Fallback order**: self-hosted FxEmbed (if configured) → `api.fxtwitter.com` → `api.vxtwitter.com`. Self-hosted goes first since it's operator-controlled and presumably more reliable/faster once deployed.
- **Parser reuse**: FxEmbed base URL is parsed with the existing `parseFxTwitter` function (same response schema as fxtwitter.com). No new parser needed.
- **URL construction**: the configured base URL is used exactly as the two existing constants are used — `base + "/" + statusID`, so the env var should be the full `https://your-domain.example/status` equivalent prefix (i.e., include `/status`) OR normalize by appending `/status` if not present. Decision: require the env var to be the bare host (e.g. `https://fx.example.com`) and append `/status` in code, matching FxEmbed's own routing (`/status/:id`) and avoiding operator error — mirrors the two existing constants' shape (`fxTwitterAPI = ".../status"`) but keeps the env var simpler for operators to set.

## Risks / Trade-offs

- [Self-hosted instance misconfigured/down] → Fallback to public mirrors still runs (per existing `fetchTweet` loop semantics), so no availability regression versus today.
- [Env var set to a URL with unexpected trailing slash or scheme] → Normalize (trim trailing `/`) before appending `/status/<id>`; cover with a unit test.
- [FxEmbed response schema drifts from FxTwitter's over time] → Out of scope to guard against; if it happens, self-hosted source simply returns unparseable/empty and the loop falls through to the public mirrors (existing behavior already tolerates a source returning an error or empty payload).

## Migration Plan

- No data migration. Purely additive config + code path.
- Rollout: env var unset by default, so existing deployments (prod `.env`, dev `.env.go.dev`) are unaffected until an operator opts in by setting `FXEMBED_BASE_URL` after deploying their own FxEmbed Worker.
- Rollback: unset the env var to revert to current two-mirror behavior; no code rollback needed.

## Open Questions

- None blocking; exact Go wiring point (config struct field vs. functional option into `xphotos`) to be finalized against current `config.go`/`worker/download.go` shape during implementation.

## Why

`go/internal/xphotos/downloader.go` resolves X/Twitter tweet media via two hardcoded public mirrors: `api.fxtwitter.com` and `api.vxtwitter.com`. Both are third-party shared services with no SLA — outages or rate limits on them directly break X media downloads for saveinator with no fallback beyond the second public mirror. FxEmbed (https://docs.fxembed.com) is the actively maintained successor to FxTwitter and can be self-hosted on Cloudflare Workers, exposing the same JSON response shape as `api.fxtwitter.com`. Adding support for an optional self-hosted FxEmbed instance as the first-tried source gives control over rate limits/uptime while keeping the public mirrors as fallback.

## What Changes

- Add a third, optional tweet-resolution source: a self-hosted FxEmbed instance, configured via a new env var (e.g. `FXEMBED_BASE_URL`).
- When configured, `fetchTweet` tries the self-hosted FxEmbed instance first (reusing the existing `parseFxTwitter` parser, since FxEmbed's `/status/:id` response is schema-compatible with FxTwitter), then falls back to `api.fxtwitter.com`, then `api.vxtwitter.com`, matching current fallback behavior.
- When the env var is unset/empty, behavior is unchanged (only the two existing public mirrors are used) — no breaking change.
- Document the new env var in `.env.example` and `CLAUDE.md` env vars table.

## Capabilities

### New Capabilities
- `x-media-resolution`: resolving tweet metadata and media URLs from X/Twitter status IDs via configurable FxTwitter-compatible API sources (self-hosted FxEmbed + public FxTwitter/VxTwitter mirrors), with ordered fallback.

### Modified Capabilities
(none — no existing specs in `openspec/specs/` cover this behavior yet)

## Impact

- Code: `go/internal/xphotos/downloader.go` (source list/fallback order), `go/internal/xphotos/downloader_test.go` (new source test coverage), `go/internal/config/config.go` (new env var), `.env.example`, `CLAUDE.md`.
- No DB/schema changes. No breaking changes — new source is additive and optional.

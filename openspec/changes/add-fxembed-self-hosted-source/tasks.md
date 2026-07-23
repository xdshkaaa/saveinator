## 1. Config

- [x] 1.1 Add `FXEMBED_BASE_URL` (optional, default empty) to `go/internal/config/config.go`
- [x] 1.2 Add `FXEMBED_BASE_URL` to `.env.example` with a comment explaining it's optional and points at a self-hosted FxEmbed Worker (see https://docs.fxembed.com/deployment/)
- [x] 1.3 Add `FXEMBED_BASE_URL` to CLAUDE.md env vars table

## 2. xphotos downloader

- [x] 2.1 Add a way to pass the configured base URL into `xphotos` (check how `worker/download.go` currently calls `xphotos.DownloadMedia`/`FetchTweetMeta` and thread it through consistently with existing patterns in this package)
- [x] 2.2 In `fetchTweet`, when a self-hosted base URL is configured, prepend it to the source list (normalized: trim trailing `/`, append `/status`), reusing `parseFxTwitter` as its parser
- [x] 2.3 Verify fallback semantics unchanged: self-hosted failure/empty payload falls through to `api.fxtwitter.com` then `api.vxtwitter.com`, matching existing `fetchTweet` loop and error aggregation

## 3. Tests

- [x] 3.1 Add test: `FXEMBED_BASE_URL` unset → only two existing public mirrors attempted (no regression)
- [x] 3.2 Add test: self-hosted source configured and returns valid payload → used directly, public mirrors not needed
- [x] 3.3 Add test: self-hosted source configured but errors/empty → falls back to public mirrors
- [x] 3.4 Add test: base URL normalization (trailing slash handling)

## 4. Verification

- [x] 4.1 `cd go && go build ./... && go test -race -count=1 ./internal/xphotos/...`
- [x] 4.2 `scripts/check-parity.sh` (if any locale/messaging touched — likely not needed for this change, confirm) — not touched, N/A

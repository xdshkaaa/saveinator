## 1. Locale

- [x] 1.1 Add `errors.callback_invalid` key (EN/RU/KK text) to `go/internal/locale/locales/{en,ru,kk}.json` (repo has no root `locales/` dir or sync script anymore — these 3 files are the single source)
- [x] 1.2 N/A — no `scripts/sync-locales.sh` exists in this repo; edited the 3 locale files directly
- [x] 1.3 Ran `scripts/check-parity.sh` — OK: all locale keys in sync

## 2. Fallback handler

- [x] 2.1 Implement `onUnknownCallback(bot *telego.Bot) func(context.Context, *telego.Bot, telego.CallbackQuery)` in `go/internal/handler/bot.go`, following the nil-guard pattern used in `onLanguageChosen` for `query.From`/`query.Message`
- [x] 2.2 In the handler: log `slog.Warn` with query ID, user ID, and raw `query.Data`, then call `bot.AnswerCallbackQuery` with the localized `errors.callback_invalid` text (use `b.userLang` if `query.From` is available, else fall back to `en`)
- [x] 2.3 Register the handler last in `Bot.Register` via `h.HandleCallbackQueryCtx(b.onUnknownCallback(bot))`, after all 9 existing prefix handlers, with a comment noting it must stay last since telego dispatches to the first matching handler

## 3. Verification

- [x] 3.1 `cd go && go build ./... && go test -race -count=1 ./...` — build succeeds, all packages pass except `internal/db` integration test (requires Docker daemon, unavailable in this environment, unrelated to this change)
- [ ] 3.2 Manually send a callback with unregistered prefix data (e.g. via `USE_POLLING=true` dev bot and a crafted inline button) and confirm the spinner clears and a warn log line appears — needs a live Telegram bot session, left for user to verify

## Why

Telego's `BotHandler` dispatches each callback query through registered prefix predicates (`lang|`, `quality:`, `ratio:`, `settings|`, `dlc:`, `dlq:`, `admin|`, `broadcast|`, `ttk:img:`) and stops at the first match. Any callback whose data does not match one of these prefixes — stale button from a bot restart, tampered/forged data, a future client sending an unrecognized payload — currently matches nothing and is silently dropped. Telegram never receives an `answerCallbackQuery`, so the user's client shows a stuck loading spinner on the button until Telegram's own timeout expires. There is no log signal either, so this failure mode is invisible to us in production.

## What Changes

- Register a catch-all callback query handler in `Bot.Register` (`go/internal/handler/bot.go`), added last so it only fires when no prefix handler matched.
- The fallback handler answers the callback query immediately with a short, localized error toast (`ShowAlert` or plain text) so the user's client stops spinning, and logs a `slog.Warn` with the query ID, user ID, and raw callback data for observability.
- Add a locale string (`callback.invalid` or similar) in all 4 locale files for the toast text.

## Capabilities

### New Capabilities
- `telegram-callback-handling`: Defines how the bot dispatches Telegram callback queries, including the fallback behavior for callback data that does not match any registered handler prefix.

### Modified Capabilities
(none — no existing capability specs cover callback dispatch yet)

## Impact

- `go/internal/handler/bot.go` — add fallback handler registration and implementation.
- `locales/en.json`, `locales/ru.json`, `go/internal/locale/locales/en.json`, `go/internal/locale/locales/ru.json` — new locale key, kept in sync via `scripts/check-parity.sh` / `scripts/sync-locales.sh`.
- No DB schema, queue, or runtime-settings changes.

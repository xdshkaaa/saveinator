## Context

`sender.Telegram` exposes `EditMessage(chatID, messageID, text)` and `EditMessageMarkup(chatID, messageID, text, markup)` (`go/internal/sender/telegram.go:44-68`). Neither sets `ParseMode`. The initial download-status message is sent by `handler/bot.go:273` with `.WithParseMode(telego.ModeHTML)`, so its `<tg-emoji>` renders. The worker then re-edits that same message to `download.downloading` at:

- `worker/download.go:195` (YouTube/generic runDownload)
- `worker/download.go:397` (Pinterest runPinterest)
- `worker/tiktok.go:26`

Those edits go through `sender.EditMessage`, dropping HTML, so the tag shows raw. `download.downloading` is the **only** worker-edited locale string containing a `<tg-emoji>` tag; every other worker edit (`userFacingError`, `pinterest.no_media`, `download.cancelled`, `*.download_track`) is plain text and could contain unescaped `<`/`>`/`&`.

## Goals / Non-Goals

**Goals:**
- Premium 🔄 emoji renders when the worker edits the status message, matching the initial send.
- Zero behavior change for plain-text edits (error strings, filenames must not break).

**Non-Goals:**
- Reworking parse-mode handling globally or auto-detecting `<tg-emoji>` inside `EditMessage`.
- Touching send/caption paths — video already sends correctly.
- Locale or schema changes.

## Decisions

- **Add a dedicated `EditMessageHTML(chatID, messageID, text)` method** on `sender.Telegram` that mirrors `EditMessage` but sets `ParseMode: telego.ModeHTML`. Call it only at the three `download.downloading` edit sites. This keeps the plain `EditMessage` intact for arbitrary/unescaped text.
  - Rationale over auto-detecting the tag inside `EditMessage`: explicit call sites are clearer, avoid a substring scan on every edit, and prevent HTML mode from ever applying to error text that happens to contain `<`.
- **Extend the `worker.deps.go` sender interface** with the new method so the worker can call it and tests can fake it.
- Leave `EditMessageMarkup` as-is: no worker path edits a `<tg-emoji>` string with a keyboard (the status message is edited without a keyboard after the initial send). If one is later added, add an HTML+markup variant then.

## Risks / Trade-offs

- **Interface surface grows by one method.** Minor; symmetric with the existing send helpers.
- **A future `<tg-emoji>` string edited via plain `EditMessage` would silently break again.** Mitigated by the spec requirement and by `download.downloading` being the sole such worker string today; a test asserting the status edit uses HTML mode guards the known path.

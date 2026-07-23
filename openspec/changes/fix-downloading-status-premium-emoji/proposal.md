## Why

The premium-emoji work wired `parse_mode: HTML` only into the handler/botkit call sites that first *send* a message. The worker's status-progress edit path (`sender.EditMessage` / `EditMessageMarkup`) sets no parse mode. So the `download.downloading` string — `<tg-emoji emoji-id="5345906554510012647">🔄</tg-emoji> Скачиваю...` — is first sent correctly (rendered emoji) by `handler/bot.go`, then re-edited by the download worker as **raw literal text**, showing the `<tg-emoji>…</tg-emoji>` tag to the user instead of the animated emoji. The video itself downloads and sends fine; only the status message is broken.

## What Changes

- Add an HTML-aware edit path to the `sender.Telegram` helper so a status message carrying a `<tg-emoji>` tag renders correctly when edited by the worker, matching how it was first sent.
- Use that HTML-aware edit for every worker call site that edits a message to `download.downloading` (the only worker-edited locale string that currently carries a `<tg-emoji>` tag): `worker/download.go` (YouTube + Pinterest paths), `worker/tiktok.go`.
- Keep the plain (non-HTML) edit for all other worker edits — `userFacingError` output, `pinterest.no_media`, `download.cancelled`, `*.download_track`, etc. — because those strings are plain text and may contain unescaped `<`, `>`, `&` from error messages or filenames that would break under HTML parse mode.

## Capabilities

### New Capabilities
<!-- none -->

### Modified Capabilities
- `premium-emoji-messaging`: extend the "premium emoji in messages" requirement to cover the **status-progress edit path**, not only the initial send. A message whose text carries a `<tg-emoji>` tag SHALL be edited with `parse_mode: HTML` too, so the rendered emoji survives subsequent edits; edits of plain-text strings SHALL remain non-HTML.

## Impact

- Code: `go/internal/sender/telegram.go` (new HTML edit method), `go/internal/worker/download.go`, `go/internal/worker/tiktok.go`.
- Behavior: status message "Скачиваю…" renders the premium 🔄 emoji throughout the download, instead of showing raw markup mid-flight.
- No schema, API, locale, or dependency changes. No breaking changes.

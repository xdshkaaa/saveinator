## Why

Bot messages that precede inline keyboards (settings menu, quality/ratio pickers, broadcast admin panel, download cards) currently render with plain unicode emoji or no emoji, using default (non-HTML) parse mode. The user wants these upgraded to Telegram Premium custom emoji per the `telegram-premium-emoji` skill, to give the bot a distinct, branded look instead of stock Telegram glyphs.

Scope note: Telegram's Bot API has no `icon_custom_emoji_id` (or equivalent) field on `InlineKeyboardButton` — verified against the `mymmrac/telego` v0.32.0 struct definitions and the Bot API reference; that field only exists for forum-topic icons. So this change covers **message text only** (captions/bodies sent alongside an inline keyboard) via the `<tg-emoji>` HTML entity. Inline button labels stay plain text — there is no API surface to attach a custom emoji icon to a button.

## What Changes

- Add `parse_mode: HTML` to `SendMessage`/`EditMessageText` calls whose locale string will carry a `<tg-emoji>` tag, wherever that message is paired with an inline keyboard.
- Update the relevant locale entries (EN + RU, canonical `locales/` and Go-embedded `go/internal/locale/locales/`) to wrap the leading emoji glyph in each message in a `<tg-emoji emoji-id="...">` tag, picking IDs from the `telegram-premium-emoji` skill's catalog by semantic match (settings→gear, lock/unlock for privacy toggles, checkmark/cross for confirmations, etc.).
- Escape any HTML-significant characters (`<`, `>`, `&`) in dynamic `{var}` substitutions for locale strings that switch to HTML parse mode, so user-controlled or dynamic values (filenames, usernames, URLs) can't break the tag or inject markup.
- Leave callback-answer/toast text (`AnswerCallbackQuery` with `ShowAlert`) as plain unicode — Telegram doesn't render HTML there, confirmed in the skill's guidance.

## Capabilities

### New Capabilities
- `premium-emoji-messaging`: Bot messages sent alongside an inline keyboard use HTML parse mode and `<tg-emoji>` tags for their leading/status emoji, with correct escaping of dynamic content and IDs matched by semantic meaning from the maintained catalog.

### Modified Capabilities
(none — no existing spec captures messaging/locale formatting behavior)

## Impact

- Go code: `go/internal/handler/{settings,admin,broadcast,music,bot}.go`, `go/internal/botkit/{admin,bot,broadcast,settings}.go`, `go/internal/sender/telegram.go`, `go/internal/tiktok/keyboard.go`, `go/internal/youtube/keyboards.go`, `go/internal/soundcloud/card.go`, `go/internal/spotify/card.go`, `go/internal/worker/{deps,music}.go`, `go/internal/botkit/platforms/{soundcloud,spotify}/{platform,worker}.go`, `go/internal/cancel/keyboard.go` — anywhere a `SendMessage`/`EditMessageText` call attaches `ReplyMarkup` with an `InlineKeyboardMarkup`.
- Locales: `locales/en.json`, `locales/ru.json`, and their Go-embedded copies under `go/internal/locale/locales/` — must stay in sync per `scripts/check-parity.sh` / `scripts/sync-locales.sh`.
- No schema, queue, or runtime-setting changes. No breaking changes to callback data or button behavior.

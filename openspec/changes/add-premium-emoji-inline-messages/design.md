## Context

Bot text today is sent with the default (non-HTML, non-Markdown) parse mode across every handler — confirmed no `ParseMode`/`WithParseMode` call exists anywhere in `go/internal/`. Locale strings live in `locales/en.json` + `locales/ru.json` (canonical) and are mirrored into `go/internal/locale/locales/` for `//go:embed`, with `{var}` placeholder substitution done by `locale.Get(key, lang, vars)`.

Telegram's `<tg-emoji emoji-id="...">fallback</tg-emoji>` tag only renders when the message is sent with `parse_mode: HTML`. Confirmed via `mymmrac/telego` v0.32.0 (`InlineKeyboardButton` struct, `types.go`) that there is no button-level custom-emoji-icon field in this Bot API binding — matches the official Bot API, where `icon_custom_emoji_id` exists only on forum-topic objects. So the only real surface for this change is message text/captions, not button labels.

## Goals / Non-Goals

**Goals:**
- Render premium custom emoji in the message text that accompanies inline keyboards (settings menu, quality/ratio pickers, broadcast admin panel, download result cards).
- Keep locale files (4-way: en/ru × canonical/embedded) in parity per existing tooling (`scripts/check-parity.sh`).
- Prevent HTML injection from dynamic `{var}` values once a locale string switches to HTML parse mode.

**Non-Goals:**
- Adding custom emoji icons to inline/reply keyboard buttons — not supported by the Bot API, out of scope.
- Reworking callback-answer/alert text — Telegram doesn't parse HTML in `AnswerCallbackQuery`, so those stay plain unicode.
- Redesigning keyboard layouts or callback data — button behavior is unchanged, only the message text above/around them.

## Decisions

- **Per-message parse mode, not global default**: Only messages that gained a `<tg-emoji>` tag switch to `ParseMode: telego.ModeHTML`. Messages with no emoji stay on default parse mode, so no risk of accidentally HTML-escaping unrelated plain-text messages that may contain literal `<`/`>` from filenames or user input. Alternative considered: set HTML as the bot-wide default — rejected, since every existing plain-text call site would then need auditing for stray `<`/`>`/`&` today, which is a much larger and riskier surface than touching only the call sites this change actually modifies.
- **Escape dynamic substitutions at the call site, not in `locale.Get`**: `locale.Get` does simple `{var}` string replacement with no awareness of whether the resulting string will be sent as HTML. Escaping (`html.EscapeString` on the substituted value) happens in the handler/worker code right before building the `SendMessage`/`EditMessageText` call, only for the messages switched to HTML mode. Alternative considered: add an HTML-aware variant inside `locale` package — rejected as unnecessary abstraction for the handful of call sites actually affected in this change; can revisit if the pattern repeats broadly later.
- **Emoji ID selection**: pull from the `telegram-premium-emoji` skill's `references/emoji-catalog.md` by semantic match to each message's meaning (e.g., settings menu → gear icon, quality/ratio picker → resolution/format icon, broadcast → megaphone, success confirmation → checkmark). Where no catalog entry fits, flag in tasks.md rather than guessing an ID — a wrong ID silently fails to render or throws `CUSTOM_EMOJI_INVALID`.
- **Locale JSON storage**: the `<tg-emoji>` tag is written directly into the locale string value (both en/ru), same as any other literal markup already embedded in strings using `<b>` tags today in some handlers. No new locale schema needed.

## Risks / Trade-offs

- [Risk] A locale string carries a `{var}` placeholder that lands inside or adjacent to the `<tg-emoji>` tag, and an unescaped value breaks the tag structure → Mitigation: escape all substituted values for any locale key touched by this change before calling `SendMessage`, and keep the tag itself outside of where `{var}` substitution occurs (tag wraps a static fallback glyph, never a variable).
- [Risk] Four locale files drift out of sync (only editing canonical or only editing embedded copies) → Mitigation: edit `locales/en.json` + `locales/ru.json` first, run `scripts/sync-locales.sh` to copy into `go/internal/locale/locales/`, then run `scripts/check-parity.sh` before considering the change done.
- [Risk] `go build` succeeds but the tag doesn't render because a call site was missed switching `ParseMode` to HTML → Mitigation: tasks.md enumerates every call site by file, and manual verification (`scripts/run-go-dev.sh` + sending `/settings` etc. in the dev bot) checks actual rendering, not just compilation.

## Migration Plan

No data migration. Deploy is a normal code + locale change: build, run locale parity check, deploy via existing `./scripts/deploy.sh`. Rollback is a normal revert — no schema or stored-state impact.

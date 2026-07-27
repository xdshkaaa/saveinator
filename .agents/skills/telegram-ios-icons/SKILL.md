---
name: telegram-ios-icons
description: Insert premium custom emoji from "Telegram iOS Icons" into Telegram content. Use when writing Telegram постов и сообщений в Telegram that should include premium emoji, or when the user mentions "Telegram iOS Icons" emoji.
---

# Telegram iOS Icons — Telegram premium emoji

## How to insert

- Requires `parse_mode=HTML` on the Telegram message.
- Syntax: `<tg-emoji emoji-id="CUSTOM_EMOJI_ID">FALLBACK</tg-emoji>`
- `FALLBACK` must be the plain emoji character shown to non-premium viewers or clients that don't support custom emoji.
- Custom emoji render only when sent from a Telegram Premium account (or an eligible channel). Regular bots generally cannot send them via the Bot API — this skill targets user-authored content, not bot replies.

## Where these are used

постов и сообщений в Telegram

## Catalog

398 emoji, fallback + id + ready snippet: [references/emoji-catalog.md](references/emoji-catalog.md)

Pick by fallback character, then copy the emoji-id exactly — IDs are long numeric strings and must not be altered.

## Code modules

Ready-to-copy maps of fallback → emoji IDs with a `tg-emoji` helper:

- Go: [references/lib/emoji.go](references/lib/emoji.go)
- Python: [references/lib/emoji.py](references/lib/emoji.py)
- TypeScript: [references/lib/emoji.ts](references/lib/emoji.ts)

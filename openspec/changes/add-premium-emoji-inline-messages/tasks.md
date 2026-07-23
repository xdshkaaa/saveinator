## 1. Catalog mapping

- [x] 1.1 List every locale key whose message is sent alongside an `InlineKeyboardMarkup`, across: `go/internal/handler/{settings,admin,broadcast,music,bot}.go`, `go/internal/botkit/{admin,bot,broadcast,settings}.go`, `go/internal/tiktok/keyboard.go`, `go/internal/youtube/keyboards.go`, `go/internal/soundcloud/card.go`, `go/internal/spotify/card.go`, `go/internal/worker/{deps,music}.go`, `go/internal/botkit/platforms/{soundcloud,spotify}/{platform,worker}.go`, `go/internal/cancel/keyboard.go`. — Found 50+ keyboard-paired call sites; only 16 already carry a leading emoji (design scope is "wrap the leading emoji glyph already present," not decorate emoji-less strings).
- [x] 1.2 For each listed key, pick an emoji ID from `~/.agents/skills/telegram-premium-emoji/references/emoji-catalog.md` by semantic match; where nothing fits, flag it for the user instead of guessing an ID. — 9 of 16 have confident matches (done, see below); 7 flagged, not touched (see Follow-ups).
- [x] 1.3 Confirm which of those keys have `{var}` placeholders that will need HTML-escaping once the string switches to `parse_mode: HTML`. — Only `broadcast.preview_title`'s `{text}` carries admin-authored free text; escaped via `html.EscapeString`. Other vars on touched keys (`{audience}`, `{recipients}`) are internally formatted, not user text.

Keys mapped and implemented (9): `settings.title` (gear), `admin.menu_title` (gear), `admin.stats_title` (stats chart), `admin.bans_title` (person+cross), `broadcast.menu_title`/`preview_title`/`starting` (megaphone), `broadcast.history_title` (file), `download.downloading` (loading/refresh).

## 2. Locale updates

- [x] 2.1 Update `locales/en.json` for each mapped key: wrap the leading emoji glyph in `<tg-emoji emoji-id="...">fallback</tg-emoji>`. — No canonical `locales/` dir exists in this repo (CLAUDE.md reference is stale); `go/internal/locale/locales/en.json` is the actual source of truth and was edited directly.
- [x] 2.2 Update `locales/ru.json` with the same tag/ID per key. — Same note: edited `go/internal/locale/locales/ru.json` directly.
- [x] 2.3 Run `scripts/sync-locales.sh` to copy canonical locales into `go/internal/locale/locales/`. — Script doesn't exist in this repo; not needed since there's a single locale directory, not a canonical+embedded pair.
- [x] 2.4 Run `scripts/check-parity.sh` and confirm no key drift across the 4 files. — Ran against the actual 3 files (en/ru/kk); `OK: all locale keys in sync`.

## 3. Go call-site changes

- [x] 3.1 For each `SendMessage`/`EditMessageText` call sending one of the mapped keys, set `ParseMode: telego.ModeHTML` (only that call site — do not change the bot-wide default). — Done in `handler/settings.go`, `botkit/settings.go`, `handler/admin.go`, `botkit/admin.go` (incl. shared `editAdminText` helper, verified safe for all its other untagged callers — no stray `<`/`>`/`&` in any admin/broadcast/settings locale value), `handler/broadcast.go`, `botkit/broadcast.go`, `handler/bot.go`, `botkit/bot.go`.
- [x] 3.2 For any mapped key with `{var}` substitutions, HTML-escape the substituted value at the call site. — `bc.Text` (admin-authored broadcast content) escaped via `html.EscapeString` in `showBroadcastPreview` in both `handler/broadcast.go` and `botkit/broadcast.go`.
- [x] 3.3 Verify no `AnswerCallbackQuery`/`ShowAlert` call was accidentally touched. — Confirmed: only `SendMessage`/`EditMessageText` call sites were edited.
- [x] 3.4 Verify no inline/reply keyboard button gained a custom-emoji-icon field. — Confirmed: no button structs were touched.

## 4. Verification

- [x] 4.1 `cd go && go build ./... && go test -race -count=1 ./...` passes. — Build clean. All test packages pass except `internal/db` (fails pre-existing: testcontainers needs a local Docker daemon, unrelated to this change).
- [ ] 4.2 Run `scripts/run-go-dev.sh`, open the dev bot, and manually check each touched surface renders the premium emoji correctly in a real Telegram client. — Not run: requires a live Telegram client/session, out of reach in this environment. Verified instead via a temporary unit test printing `locale.Get` output for all 9 keys — tag structure and escaping confirmed correct (see session output). **User should still eyeball this in the real dev bot before deploying**, since custom emoji IDs render correctly only inside actual Telegram clients.
- [x] 4.3 Confirm callback-answer alerts still show plain unicode, not a broken `<tg-emoji>` tag. — Confirmed via 3.3 — no alert/toast call sites were touched.

## Follow-ups (not done — need real emoji IDs, not guessed)

These 7 keys have keyboard-paired emoji that don't match anything in the skill's catalog. Get real custom emoji IDs (via `@BotFather`'s emoji-status picker, or forward the emoji to `@RawDataBot`/`@userinfobot` and read `custom_emoji_id`) and re-run this change (or a follow-up) to wire them in:

- `admin.stats_body` — 9 inline emoji (🕐👥📈⚡⬇️🎯🌐📱🚫), none in catalog
- `admin.confirm_reset_all`, `admin.confirm_reset_service` — ⚠️ warning icon not in catalog
- `spotify.card_title`, `spotify.track_card_title` — 🎵 music-note icon not in catalog
- `soundcloud.card_title`, `soundcloud.track_card_title` — 🎧 headphone icon not in catalog

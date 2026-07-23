## ADDED Requirements

### Requirement: Premium emoji in messages paired with inline keyboards
When the bot sends or edits a message that attaches an `InlineKeyboardMarkup` (settings menu, quality/ratio picker, broadcast admin panel, download result card, cancel confirmation), the message text SHALL use `parse_mode: HTML` and SHALL render its leading/status emoji via a `<tg-emoji emoji-id="...">fallback</tg-emoji>` tag instead of a bare unicode emoji.

#### Scenario: Settings menu opened
- **WHEN** a user sends `/settings` (or presses the settings entry point) and the bot sends the settings menu message with its inline keyboard
- **THEN** the message is sent with `parse_mode: HTML` and its emoji is wrapped in a `<tg-emoji emoji-id="...">` tag matching the settings/gear icon in the maintained catalog

#### Scenario: Quality or ratio picker shown
- **WHEN** the bot sends the quality-selection or ratio-selection message alongside its inline keyboard
- **THEN** the message text's leading emoji renders via `<tg-emoji>` with an ID semantically matched to that picker (e.g., resolution/format icon)

#### Scenario: Broadcast admin panel shown
- **WHEN** an admin opens the broadcast composer message with its inline keyboard
- **THEN** the message text's emoji renders via `<tg-emoji>` with the megaphone icon ID

### Requirement: Callback-answer alerts stay plain unicode
Toast/alert responses sent via `AnswerCallbackQuery` (with or without `ShowAlert`) SHALL NOT be switched to HTML parse mode and SHALL keep plain unicode emoji, since Telegram does not render HTML markup in that surface.

#### Scenario: Unauthorized admin action
- **WHEN** a non-admin user presses a callback button gated to admins
- **THEN** the alert text shown via `AnswerCallbackQuery` uses plain unicode emoji (e.g. `✖️`), not a `<tg-emoji>` tag

### Requirement: Dynamic values are HTML-escaped before substitution
For any locale string modified by this change, dynamic `{var}` values substituted into that string SHALL be HTML-escaped before being sent, so that user-controlled or otherwise dynamic content (filenames, usernames, counts) cannot break the `<tg-emoji>` tag or inject markup.

#### Scenario: Broadcast preview includes a user-supplied caption
- **WHEN** the broadcast message text includes a dynamic value that itself contains `<`, `>`, or `&`
- **THEN** that value is HTML-escaped in the outgoing message so the tag structure and rendered text remain intact

### Requirement: Inline keyboard buttons remain unchanged
Inline and reply keyboard button labels SHALL remain plain text with no custom-emoji-icon attribute, since the Bot API exposes no such field on keyboard buttons (only on forum-topic objects).

#### Scenario: Any inline keyboard button
- **WHEN** an inline keyboard is built for any handler touched by this change
- **THEN** its buttons carry only `text`/`callback_data`/`url` (or other existing fields) and no custom-emoji-icon field is introduced

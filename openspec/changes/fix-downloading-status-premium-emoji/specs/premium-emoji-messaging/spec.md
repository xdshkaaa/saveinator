## ADDED Requirements

### Requirement: Premium emoji survives status-progress edits
When the bot edits an existing message to a locale string that carries a `<tg-emoji emoji-id="...">fallback</tg-emoji>` tag (the download status text re-set by the worker during a download), the edit SHALL use `parse_mode: HTML` so the premium emoji renders, matching the parse mode used when the message was first sent. Edits to plain-text locale strings that contain no `<tg-emoji>` tag SHALL NOT be switched to HTML parse mode, since those strings may contain unescaped `<`, `>`, or `&` from error text or filenames.

#### Scenario: Worker re-sets the download status message
- **WHEN** a download task begins and the worker edits the existing status message to the `download.downloading` string (which contains a `<tg-emoji>` tag)
- **THEN** the edit is performed with `parse_mode: HTML` and the user sees the rendered premium 🔄 emoji, not the literal `<tg-emoji>…</tg-emoji>` markup

#### Scenario: Worker edits the status message to an error string
- **WHEN** a download fails and the worker edits the status message to a user-facing error string that carries no `<tg-emoji>` tag and may include raw `<`, `>`, or `&`
- **THEN** the edit is performed without HTML parse mode, so the error text is shown verbatim and no entity-parse failure occurs

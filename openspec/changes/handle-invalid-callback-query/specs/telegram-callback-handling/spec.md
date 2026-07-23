## ADDED Requirements

### Requirement: Unmatched callback queries are acknowledged
The bot SHALL respond to every incoming Telegram callback query with an `answerCallbackQuery` call, including callback queries whose `data` does not match any registered handler prefix (`lang|`, `quality:`, `ratio:`, `settings|`, `dlc:`, `dlq:`, `admin|`, `broadcast|`, `ttk:img:`).

#### Scenario: Callback data matches no known prefix
- **WHEN** a callback query arrives with `data` that does not start with any registered prefix (e.g. a stale button from before a bot restart, or a forged/tampered payload)
- **THEN** the bot calls `answerCallbackQuery` for that query with a short localized error message
- **AND** the user's Telegram client stops showing the loading spinner on the button

### Requirement: Unmatched callback queries are logged
The bot SHALL log a warning-level event whenever a callback query does not match any registered handler, including the callback query ID, the originating user ID, and the raw callback data.

#### Scenario: Unmatched callback is logged for observability
- **WHEN** the fallback callback handler is invoked because no prefix handler matched
- **THEN** the bot logs at warn level with fields identifying the query ID, user ID, and raw callback data
- **AND** the bot does not panic or crash while handling the unmatched callback

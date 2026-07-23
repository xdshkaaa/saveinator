## ADDED Requirements

### Requirement: Configurable self-hosted FxEmbed source
The system SHALL support an optional, operator-configured self-hosted FxEmbed base URL (via env var `FXEMBED_BASE_URL`) as an additional tweet-resolution source.

#### Scenario: Env var unset
- **WHEN** `FXEMBED_BASE_URL` is not set (or empty)
- **THEN** tweet resolution uses only the existing `api.fxtwitter.com` then `api.vxtwitter.com` sources, in that order, with no change to current behavior

#### Scenario: Env var set
- **WHEN** `FXEMBED_BASE_URL` is set to a valid host (e.g. `https://fx.example.com`)
- **THEN** tweet resolution tries the self-hosted FxEmbed instance's `/status/<id>` endpoint first, before `api.fxtwitter.com` and `api.vxtwitter.com`

### Requirement: Fallback on self-hosted source failure
The system SHALL fall back to the existing public mirrors if the configured self-hosted FxEmbed source errors, times out, or returns an empty tweet payload.

#### Scenario: Self-hosted instance unreachable
- **WHEN** the self-hosted FxEmbed source is configured but the request fails (network error, timeout, or HTTP >= 400)
- **THEN** the system proceeds to try `api.fxtwitter.com`, then `api.vxtwitter.com`, and returns a successful result if either succeeds

#### Scenario: Self-hosted instance returns empty payload
- **WHEN** the self-hosted FxEmbed source returns HTTP 200 with no text, author, or media
- **THEN** the system treats it as a non-match and proceeds to the next source in the fallback order

#### Scenario: All sources fail
- **WHEN** the self-hosted source (if configured) and both public mirrors all fail or return empty
- **THEN** the system returns `ErrNotFound` wrapping the combined error details from all attempted sources, matching current error-aggregation behavior

### Requirement: Schema-compatible parsing
The system SHALL parse the self-hosted FxEmbed `/status/<id>` response using the same parser used for `api.fxtwitter.com`, since FxEmbed's response schema is API-compatible with FxTwitter.

#### Scenario: Self-hosted response parsed successfully
- **WHEN** the self-hosted FxEmbed source returns a valid FxTwitter-shaped JSON payload (tweet text, author, media)
- **THEN** the system extracts text, author, and media items identically to how it would from `api.fxtwitter.com`

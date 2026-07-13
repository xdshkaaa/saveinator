## ADDED Requirements

### Requirement: Shared-secret authentication on internal download endpoints
The system SHALL require a valid `X-Internal-Token` header matching the configured `INTERNAL_API_TOKEN` on every request to `POST /download/pinterest` before processing the request body.

#### Scenario: Missing token
- **WHEN** a request to `POST /download/pinterest` has no `X-Internal-Token` header
- **THEN** the system responds `401 Unauthorized` and does not parse the request body or invoke the Pinterest client

#### Scenario: Incorrect token
- **WHEN** a request to `POST /download/pinterest` has an `X-Internal-Token` header that does not match `INTERNAL_API_TOKEN`
- **THEN** the system responds `401 Unauthorized` and does not parse the request body or invoke the Pinterest client

#### Scenario: Valid token
- **WHEN** a request to `POST /download/pinterest` has an `X-Internal-Token` header matching `INTERNAL_API_TOKEN`
- **THEN** the system processes the request as before this change (existing validation, download, and response behavior is unchanged)

### Requirement: Per-IP rate limiting on internal download endpoints
The system SHALL apply a per-IP rate limit to `POST /download/pinterest`, rejecting requests over the limit with `429 Too Many Requests`, independent of and in addition to token authentication.

#### Scenario: Under the limit
- **WHEN** a client IP has made fewer requests than the configured limit within the current window
- **THEN** the request proceeds to token authentication and normal handling

#### Scenario: Over the limit
- **WHEN** a client IP has exceeded the configured request limit within the current window
- **THEN** the system responds `429 Too Many Requests` regardless of whether a valid token is present

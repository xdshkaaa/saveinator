import time

from prometheus_client import Counter, Gauge, Histogram

_START_TIME = time.monotonic()

UPTIME_SECONDS = Gauge(
    "saveinator_uptime_seconds",
    "Bot process uptime in seconds",
)

MESSAGES_RECEIVED_TOTAL = Counter(
    "saveinator_messages_received_total",
    "Telegram messages received",
)

COMMANDS_HANDLED_TOTAL = Counter(
    "saveinator_commands_handled_total",
    "Bot commands handled",
    ["command"],
)

ERRORS_TOTAL = Counter(
    "saveinator_errors_total",
    "Unhandled errors and failures",
    ["source"],
)

TELEGRAM_API_REQUESTS_TOTAL = Counter(
    "saveinator_telegram_api_requests_total",
    "Telegram Bot API requests",
    ["method", "status"],
)

TELEGRAM_API_LATENCY_SECONDS = Histogram(
    "saveinator_telegram_api_latency_seconds",
    "Telegram Bot API request latency",
    ["method"],
    buckets=(0.05, 0.1, 0.25, 0.5, 1.0, 2.5, 5.0, 10.0),
)

TELEGRAM_API_FAILURES_TOTAL = Counter(
    "saveinator_telegram_api_failures_total",
    "Failed Telegram Bot API requests",
)

ACTIVE_CHATS = Gauge(
    "saveinator_active_chats",
    "Recently active unique chat IDs",
)

DOWNLOADS_ENQUEUED_TOTAL = Counter(
    "saveinator_downloads_enqueued_total",
    "Video downloads enqueued to Celery",
    ["platform"],
)

SPOTIFY_REQUESTS_TOTAL = Counter(
    "saveinator_spotify_requests_total",
    "Spotify link handling requests",
)

RATE_LIMIT_DROPPED_TOTAL = Counter(
    "saveinator_rate_limit_dropped_total",
    "Messages dropped by rate limiter",
    ["scope"],
)

USER_QUEUE_REJECTED_TOTAL = Counter(
    "saveinator_user_queue_rejected_total",
    "Download requests rejected because the user already has an active scenario",
    ["scenario"],
)

SPAM_BLOCKED_TOTAL = Counter(
    "saveinator_spam_blocked_total",
    "Messages blocked by spam middleware",
    ["reason"],
)

_active_chat_ids: set[int] = set()


def refresh_uptime() -> None:
    UPTIME_SECONDS.set(time.monotonic() - _START_TIME)


def record_message(chat_id: int | None) -> None:
    MESSAGES_RECEIVED_TOTAL.inc()
    if chat_id is not None:
        _active_chat_ids.add(chat_id)
        ACTIVE_CHATS.set(len(_active_chat_ids))


def record_command(command: str) -> None:
    COMMANDS_HANDLED_TOTAL.labels(command=command).inc()


def record_error(source: str) -> None:
    ERRORS_TOTAL.labels(source=source).inc()

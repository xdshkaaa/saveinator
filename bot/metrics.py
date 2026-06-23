import time

from prometheus_client import Counter, Gauge, Histogram

_START_TIME = time.monotonic()
_ACTIVE_CHAT_WINDOW = 1800  # 30 minutes in seconds

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

HTTP_REQUESTS_TOTAL = Counter(
    "saveinator_http_requests_total",
    "HTTP API requests by method, route, and status",
    ["method", "route", "status"],
)

HTTP_REQUEST_LATENCY_SECONDS = Histogram(
    "saveinator_http_request_latency_seconds",
    "HTTP API request latency by method and route",
    ["method", "route"],
    buckets=(0.01, 0.025, 0.05, 0.1, 0.25, 0.5, 1.0, 2.5, 5.0, 10.0, 30.0),
)

ACTIVE_CHATS = Gauge(
    "saveinator_active_chats",
    "Recently active unique chat IDs",
)

USERS_CREATED_TOTAL = Counter(
    "saveinator_users_created_total",
    "Users created in the Saveinator database",
)

DOWNLOAD_PLATFORMS = (
    "youtube",
    "tiktok",
    "instagram",
    "x",
    "pinterest",
    "spotify",
    "soundcloud",
)

DOWNLOADS_ENQUEUED_TOTAL = Counter(
    "saveinator_downloads_enqueued_total",
    "Download jobs started by platform (Celery video/Pinterest, inline Spotify)",
    ["platform"],
)

SPOTIFY_REQUESTS_TOTAL = Counter(
    "saveinator_spotify_requests_total",
    "Spotify link handling requests",
)

SOUNDCLOUD_REQUESTS_TOTAL = Counter(
    "saveinator_soundcloud_requests_total",
    "SoundCloud link handling requests",
)

SOUNDCLOUD_DOWNLOADS_ENQUEUED_TOTAL = Counter(
    "saveinator_soundcloud_downloads_enqueued_total",
    "SoundCloud track downloads started",
)

SOUNDCLOUD_DOWNLOADS_SUCCESS_TOTAL = Counter(
    "saveinator_soundcloud_downloads_success_total",
    "Successful SoundCloud track downloads",
)

SOUNDCLOUD_DOWNLOAD_FAILURES_TOTAL = Counter(
    "saveinator_soundcloud_download_failures_total",
    "Failed SoundCloud track downloads",
)

SOUNDCLOUD_DOWNLOADS_TIMEOUT_TOTAL = Counter(
    "saveinator_soundcloud_download_timeouts_total",
    "SoundCloud track download timeouts",
)

SOUNDCLOUD_METADATA_FAILURES_TOTAL = Counter(
    "saveinator_soundcloud_metadata_failures_total",
    "SoundCloud metadata fetch failures",
)

SOUNDCLOUD_PLAYLIST_TRACKS = Histogram(
    "saveinator_soundcloud_playlist_tracks",
    "Number of tracks in SoundCloud playlists",
    buckets=(1, 2, 5, 10, 15, 20, 30, 50),
)

TIKTOK_CAROUSEL_REQUESTS_TOTAL = Counter(
    "saveinator_tiktok_carousel_requests_total",
    "TikTok carousel download requests",
    ["status"],
)

TIKTOK_CAROUSEL_IMAGES_TOTAL = Counter(
    "saveinator_tiktok_carousel_images_total",
    "Images sent from TikTok carousels",
)

TIKTOK_CAROUSEL_AUDIO_TOTAL = Counter(
    "saveinator_tiktok_carousel_audio_total",
    "Audio tracks sent from TikTok carousels",
)

TIKTOK_CAROUSEL_FAILURES_TOTAL = Counter(
    "saveinator_tiktok_carousel_failures_total",
    "TikTok carousel failures",
    ["reason"],
)

SOUNDCLOUD_DOWNLOAD_DURATION_SECONDS = Histogram(
    "saveinator_soundcloud_download_duration_seconds",
    "SoundCloud track download duration",
    buckets=(1, 3, 5, 10, 15, 30, 60, 120),
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

BROADCASTS_TOTAL = Counter(
    "saveinator_broadcasts_total",
    "Broadcasts created",
    ["audience"],
)

BROADCAST_MESSAGES_SENT_TOTAL = Counter(
    "saveinator_broadcast_messages_sent_total",
    "Broadcast messages successfully sent",
)

BROADCAST_MESSAGES_FAILED_TOTAL = Counter(
    "saveinator_broadcast_messages_failed_total",
    "Broadcast messages that failed to send",
)

ADMIN_RUNTIME_SETTINGS_CHANGED_TOTAL = Counter(
    "saveinator_admin_runtime_settings_changed_total",
    "Admin runtime setting changes",
    ["service"],
)

RPC_FAILURES_TOTAL = Counter(
    "saveinator_rpc_failures_total",
    "Infrastructure RPC failures (database, redis, internal)",
    ["rpc_type"],
)

_active_chat_ids: dict[int, float] = {}  # chat_id -> last_seen timestamp


def init_platform_metrics() -> None:
    for platform in DOWNLOAD_PLATFORMS:
        DOWNLOADS_ENQUEUED_TOTAL.labels(platform=platform).inc(0)


def refresh_uptime() -> None:
    UPTIME_SECONDS.set(time.monotonic() - _START_TIME)


def record_message(chat_id: int | None) -> None:
    MESSAGES_RECEIVED_TOTAL.inc()
    if chat_id is not None:
        _active_chat_ids[chat_id] = time.monotonic()


def refresh_active_chats() -> None:
    """Prune chat IDs older than _ACTIVE_CHAT_WINDOW and update the gauge.

    Called from the ``/metrics`` handler so the value is always fresh on
    scrape — no background loop needed.
    """
    now = time.monotonic()
    cutoff = now - _ACTIVE_CHAT_WINDOW
    stale = [cid for cid, last_seen in _active_chat_ids.items() if last_seen < cutoff]
    for cid in stale:
        del _active_chat_ids[cid]
    ACTIVE_CHATS.set(len(_active_chat_ids))


def record_command(command: str) -> None:
    COMMANDS_HANDLED_TOTAL.labels(command=command).inc()


def record_error(source: str) -> None:
    ERRORS_TOTAL.labels(source=source).inc()


def record_user_created() -> None:
    USERS_CREATED_TOTAL.inc()


def record_rpc_failure(rpc_type: str) -> None:
    RPC_FAILURES_TOTAL.labels(rpc_type=rpc_type).inc()

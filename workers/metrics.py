import time

from prometheus_client import Counter, Gauge, Histogram

_START_TIME = time.monotonic()

WORKER_UPTIME = Gauge(
    "saveinator_worker_uptime_seconds",
    "Celery worker process uptime in seconds",
)

CELERY_TASKS_TOTAL = Counter(
    "saveinator_celery_tasks_total",
    "Celery tasks by name and status",
    ["task", "status"],
)

DOWNLOAD_DURATION_SECONDS = Histogram(
    "saveinator_download_duration_seconds",
    "Video download task duration",
    ["platform"],
    buckets=(1, 5, 15, 30, 60, 120, 300, 600),
)

YTDLP_ERRORS_TOTAL = Counter(
    "saveinator_ytdlp_errors_total",
    "yt-dlp download failures",
    ["platform"],
)


def refresh_worker_uptime() -> None:
    WORKER_UPTIME.set(time.monotonic() - _START_TIME)

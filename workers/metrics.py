import os
import time

from prometheus_client import Counter, Gauge, Histogram

# Ensure prometheus multiproc dir exists and is clean before any metrics
# are created.  prometheus_client checks PROMETHEUS_MULTIPROC_DIR internally
# and creates a .db file there for every metric.  If the dir doesn't exist,
# metric creation fails with FileNotFoundError.
_multiproc_dir = os.environ.get("PROMETHEUS_MULTIPROC_DIR")
if _multiproc_dir:
    os.makedirs(_multiproc_dir, exist_ok=True)
    for _entry in os.listdir(_multiproc_dir):
        if _entry.endswith(".db"):
            try:
                os.remove(os.path.join(_multiproc_dir, _entry))
            except OSError:
                pass

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

DOWNLOAD_FILE_SIZE_BYTES = Histogram(
    "saveinator_download_file_size_bytes",
    "Downloaded file sizes by platform",
    ["platform"],
    buckets=(
        1 * 1024 * 1024,
        5 * 1024 * 1024,
        10 * 1024 * 1024,
        25 * 1024 * 1024,
        50 * 1024 * 1024,
        100 * 1024 * 1024,
        250 * 1024 * 1024,
        500 * 1024 * 1024,
        1024 * 1024 * 1024,
        2 * 1024 * 1024 * 1024,
    ),
)

YTDLP_PLATFORMS = ("youtube", "tiktok", "instagram", "x", "pinterest")

YTDLP_ERRORS_TOTAL = Counter(
    "saveinator_ytdlp_errors_total",
    "yt-dlp download failures",
    ["platform"],
)


def init_worker_platform_metrics() -> None:
    for platform in YTDLP_PLATFORMS:
        YTDLP_ERRORS_TOTAL.labels(platform=platform).inc(0)
        DOWNLOAD_DURATION_SECONDS.labels(platform=platform)
        DOWNLOAD_FILE_SIZE_BYTES.labels(platform=platform)


def refresh_worker_uptime() -> None:
    WORKER_UPTIME.set(time.monotonic() - _START_TIME)

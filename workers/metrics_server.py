import logging
import os
import threading

from prometheus_client import CollectorRegistry, start_http_server
from prometheus_client.multiprocess import MultiProcessCollector

from bot.config import settings
from workers.metrics import refresh_worker_uptime

logger = logging.getLogger(__name__)


def _create_multiprocess_registry() -> CollectorRegistry:
    """Create a registry that aggregates .db files from all child processes.

    Without multiprocess mode, Counter increments in Celery forked child
    processes are invisible to the parent's HTTP metrics server due to
    copy-on-write memory.  MultiProcessCollector reads ``.db`` files that
    each child writes (prometheus_client does this automatically when
    ``PROMETHEUS_MULTIPROC_DIR`` is set) and merges them on every
    ``/metrics`` scrape.

    Returns a **clean** registry that contains *only* the
    MultiProcessCollector — not the in-memory metric objects from the parent
    process (which would produce duplicate, zero-valued metric families).

    Stale ``.db`` files from previous worker generations are cleaned in
    ``workers/metrics.py`` at import time (before any metrics are created),
    so there is no need to remove them here.
    """
    multiproc_dir = os.environ.get("PROMETHEUS_MULTIPROC_DIR")
    if multiproc_dir:
        logger.info("Prometheus multiproc dir: %s", multiproc_dir)

    registry = CollectorRegistry()
    MultiProcessCollector(registry)
    return registry


def _uptime_refresh_loop() -> None:
    import time

    while True:
        refresh_worker_uptime()
        time.sleep(15)


def start_worker_metrics_server() -> None:
    if not settings.metrics_enabled:
        return
    registry = _create_multiprocess_registry()
    start_http_server(
        settings.worker_metrics_port,
        addr=settings.metrics_host,
        registry=registry,
    )
    thread = threading.Thread(target=_uptime_refresh_loop, daemon=True)
    thread.start()
    logger.info(
        "Worker metrics server listening on %s:%s",
        settings.metrics_host,
        settings.worker_metrics_port,
    )

import logging
import threading

from prometheus_client import start_http_server

from bot.config import settings
from workers.metrics import refresh_worker_uptime

logger = logging.getLogger(__name__)


def _uptime_refresh_loop() -> None:
    import time

    while True:
        refresh_worker_uptime()
        time.sleep(15)


def start_worker_metrics_server() -> None:
    if not settings.metrics_enabled:
        return
    start_http_server(settings.worker_metrics_port, addr=settings.metrics_host)
    thread = threading.Thread(target=_uptime_refresh_loop, daemon=True)
    thread.start()
    logger.info(
        "Worker metrics server listening on %s:%s",
        settings.metrics_host,
        settings.worker_metrics_port,
    )

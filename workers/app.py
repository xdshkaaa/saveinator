import asyncio
import os
import time

from celery import Celery
from celery.signals import (
    task_failure,
    task_postrun,
    task_prerun,
    worker_process_shutdown,
    worker_ready,
    worker_shutting_down,
)

from bot.config import settings
from workers.metrics import (
    CELERY_TASKS_TOTAL,
    DOWNLOAD_DURATION_SECONDS,
    init_worker_platform_metrics,
)
from workers.metrics_server import start_worker_metrics_server

app = Celery(
    "saveinator",
    broker=settings.celery_broker_url,
    backend=settings.celery_result_backend,
    include=["workers.tasks", "workers.tiktok_task", "workers.pinterest_task", "workers.broadcast_task"],
)

app.conf.update(
    task_serializer="json",
    accept_content=["json"],
    result_serializer="json",
    timezone="UTC",
    enable_utc=True,
    task_track_started=True,
    task_acks_late=True,
    worker_prefetch_multiplier=1,
    beat_schedule={
        "sweep-tempfiles": {
            "task": "workers.tasks.cleanup_stale_task",
            "schedule": 3600.0,
        },
    },
)

_task_start_times: dict[str, float] = {}


@worker_ready.connect
def _on_worker_ready(**_kwargs) -> None:
    init_worker_platform_metrics()
    start_worker_metrics_server()


@task_prerun.connect
def _on_task_prerun(task_id=None, task=None, **_kwargs) -> None:
    if task_id:
        _task_start_times[task_id] = time.monotonic()


@task_postrun.connect
def _on_task_postrun(task_id=None, task=None, state=None, kwargs=None, **_extra) -> None:
    task_name = task.name if task else "unknown"
    status = state or "success"
    CELERY_TASKS_TOTAL.labels(task=task_name, status=status).inc()

    if task_id and task_id in _task_start_times:
        duration = time.monotonic() - _task_start_times.pop(task_id)
        if task_name == "workers.tasks.download_and_send_task":
            platform = (kwargs or {}).get("platform", "unknown")
            DOWNLOAD_DURATION_SECONDS.labels(platform=platform).observe(duration)
        elif task_name == "workers.pinterest_task.pinterest_download_task":
            DOWNLOAD_DURATION_SECONDS.labels(platform="pinterest").observe(duration)


@task_failure.connect
def _on_task_failure(task_id=None, **_kwargs) -> None:
    if task_id and task_id in _task_start_times:
        _task_start_times.pop(task_id, None)


@worker_process_shutdown.connect
def _on_worker_process_shutdown(pid=None, **_kwargs) -> None:
    """Mark this child process's .db file as dead for multiprocess Prometheus."""
    from prometheus_client.multiprocess import mark_process_dead

    mark_process_dead(pid or os.getpid())


@worker_shutting_down.connect
def _on_worker_shutdown(**kwargs) -> None:
    """Close shared Bot session on worker shutdown."""
    from workers.bot import close_bot

    try:
        loop = asyncio.new_event_loop()
        try:
            loop.run_until_complete(close_bot())
        finally:
            loop.close()
    except Exception:
        pass

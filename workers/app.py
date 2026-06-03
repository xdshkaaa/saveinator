from celery import Celery

from bot.config import settings

app = Celery(
    "saveinator",
    broker=settings.celery_broker_url,
    backend=settings.celery_result_backend,
    include=["workers.tasks"],
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

import logging
from aiogram import Router
from aiogram.types import ErrorEvent

from bot.metrics import record_error

errors_router = Router()
logger = logging.getLogger(__name__)


@errors_router.errors()
async def global_error_handler(event: ErrorEvent):
    record_error("dispatcher")
    logger.exception(
        "Unhandled exception",
        extra={
            "update_id": getattr(event.update, "update_id", None),
            "exception": str(event.exception),
        },
    )

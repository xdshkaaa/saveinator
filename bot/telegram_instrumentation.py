import time
from collections.abc import Awaitable, Callable
from typing import Any

from aiogram import Bot

from bot.metrics import (
    TELEGRAM_API_FAILURES_TOTAL,
    TELEGRAM_API_LATENCY_SECONDS,
    TELEGRAM_API_REQUESTS_TOTAL,
)


def instrument_bot(bot: Bot) -> None:
    session = bot.session
    original_make_request = session.make_request

    async def make_request(
        bot_instance: Bot,
        method: Any,
        **kwargs: Any,
    ) -> Any:
        method_name = getattr(method, "__api_method__", type(method).__name__)
        start = time.perf_counter()
        try:
            result = await original_make_request(bot_instance, method, **kwargs)
            TELEGRAM_API_REQUESTS_TOTAL.labels(method=method_name, status="ok").inc()
            return result
        except Exception:
            TELEGRAM_API_FAILURES_TOTAL.inc()
            TELEGRAM_API_REQUESTS_TOTAL.labels(method=method_name, status="error").inc()
            raise
        finally:
            TELEGRAM_API_LATENCY_SECONDS.labels(method=method_name).observe(
                time.perf_counter() - start
            )

    session.make_request = make_request  # type: ignore[method-assign]

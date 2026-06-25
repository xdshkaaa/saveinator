from typing import Any, Awaitable, Callable

from aiogram import BaseMiddleware
from aiogram.types import CallbackQuery, Message, TelegramObject

from bot.metrics import record_message


class MetricsMiddleware(BaseMiddleware):
    async def __call__(
        self,
        handler: Callable[[TelegramObject, dict[str, Any]], Awaitable[Any]],
        event: TelegramObject,
        data: dict[str, Any],
    ) -> Any:
        if isinstance(event, Message):
            record_message(
                event.chat.id if event.chat else None,
                event.from_user.id if event.from_user else None,
            )
        elif isinstance(event, CallbackQuery) and event.message and event.message.chat:
            record_message(
                event.message.chat.id,
                event.from_user.id if event.from_user else None,
            )
        return await handler(event, data)

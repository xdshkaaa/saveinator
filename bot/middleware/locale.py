from typing import Any, Awaitable, Callable
from aiogram import BaseMiddleware
from aiogram.types import TelegramObject, Message, CallbackQuery
from sqlalchemy import select

from db.models import User, Language
from db.session import async_session_factory
from bot.locale import get


class LocaleMiddleware(BaseMiddleware):
    async def __call__(
        self,
        handler: Callable[[TelegramObject, dict[str, Any]], Awaitable[Any]],
        event: TelegramObject,
        data: dict[str, Any],
    ) -> Any:
        user_id: int | None = None

        if isinstance(event, Message):
            user_id = event.from_user.id if event.from_user else None
        elif isinstance(event, CallbackQuery):
            user_id = event.from_user.id if event.from_user else None

        lang = "en"
        if user_id:
            async with async_session_factory() as session:
                result = await session.execute(select(User.language).where(User.id == user_id))
                row = result.scalar_one_or_none()
                if row:
                    lang = row.value

        data["lang"] = lang
        data["_"] = lambda key, **kwargs: get(key, lang, **kwargs)

        return await handler(event, data)

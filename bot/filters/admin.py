from typing import Any

from aiogram.filters import BaseFilter
from aiogram.types import CallbackQuery, Message, TelegramObject

from bot.config import settings


class IsAdminFilter(BaseFilter):
    async def __call__(self, event: TelegramObject, *_args: Any, **_kwargs: Any) -> bool:
        user = getattr(event, "from_user", None)
        if user is None and isinstance(event, CallbackQuery):
            user = event.from_user
        return bool(user and user.id == settings.admin_telegram_id)

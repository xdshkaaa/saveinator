from typing import Any, Awaitable, Callable

from aiogram import BaseMiddleware
from aiogram.types import Message, TelegramObject

from bot.config import settings
from bot.locale import get
from bot.services.ban_notifications import notify_admin_banned_message
from bot.services.user_bans import is_user_banned


class UserBanMiddleware(BaseMiddleware):
    async def __call__(
        self,
        handler: Callable[[TelegramObject, dict[str, Any]], Awaitable[Any]],
        event: TelegramObject,
        data: dict[str, Any],
    ) -> Any:
        if not isinstance(event, Message):
            return await handler(event, data)

        user_id = event.from_user.id if event.from_user else None
        if not user_id or user_id == settings.admin_telegram_id:
            return await handler(event, data)

        if await is_user_banned(user_id):
            lang = data.get("lang", "ru")
            await event.answer(get("ban.shadow_reply", lang))
            if bot := data.get("bot"):
                await notify_admin_banned_message(bot, event, lang=lang)
            return None

        return await handler(event, data)

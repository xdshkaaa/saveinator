import time
from typing import Any, Awaitable, Callable
from aiogram import BaseMiddleware
from aiogram.types import TelegramObject, Message

from bot.config import settings
from bot.locale import get


class RateLimitMiddleware(BaseMiddleware):
    def __init__(self) -> None:
        import redis.asyncio as aioredis
        self._redis: aioredis.Redis | None = None

    async def _get_redis(self):
        if self._redis is None:
            import redis.asyncio as aioredis
            self._redis = aioredis.from_url(settings.redis_url, decode_responses=True)
        return self._redis

    async def __call__(
        self,
        handler: Callable[[TelegramObject, dict[str, Any]], Awaitable[Any]],
        event: TelegramObject,
        data: dict[str, Any],
    ) -> Any:
        if not isinstance(event, Message) or not event.text:
            return await handler(event, data)

        user_id = event.from_user.id if event.from_user else None
        chat_id = event.chat.id

        r = await self._get_redis()
        now = time.time()
        window = 60

        if user_id:
            user_key = f"ratelimit:user:{user_id}"
            await r.zremrangebyscore(user_key, 0, now - window)
            user_count = await r.zcard(user_key)
            if user_count >= settings.rate_limit_user_per_minute:
                lang = data.get("lang", "en")
                if event.chat.type == "private":
                    await event.answer(
                        get("errors.rate_limit", lang,
                            count=settings.rate_limit_user_per_minute,
                            window=window)
                    )
                return None
            await r.zadd(user_key, {str(now): now})
            await r.expire(user_key, window + 10)

        chat_key = f"ratelimit:chat:{chat_id}"
        await r.zremrangebyscore(chat_key, 0, now - window)
        chat_count = await r.zcard(chat_key)
        if chat_count >= settings.rate_limit_chat_per_minute:
            return None
        await r.zadd(chat_key, {str(now): now})
        await r.expire(chat_key, window + 10)

        return await handler(event, data)

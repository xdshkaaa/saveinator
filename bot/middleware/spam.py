import hashlib
import time
from typing import Any, Awaitable, Callable
from aiogram import BaseMiddleware
from aiogram.types import TelegramObject, Message
from sqlalchemy import select

from bot.config import settings
from bot.metrics import SPAM_BLOCKED_TOTAL
from db.models import BannedLink
from db.session import async_session_factory


class SpamMiddleware(BaseMiddleware):
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

        if event.chat.type == "private":
            return await handler(event, data)

        import re
        urls = re.findall(r'https?://\S+', event.text)
        for url in urls:
            url_hash = hashlib.sha256(url.encode()).hexdigest()

            async with async_session_factory() as session:
                banned = await session.scalar(
                    select(BannedLink.id).where(BannedLink.url_hash == url_hash[:64])
                )
                if banned:
                    SPAM_BLOCKED_TOTAL.labels(reason="banned").inc()
                    return None

            r = await self._get_redis()
            dedup_key = f"dedup:{url_hash[:12]}"
            if await r.exists(dedup_key):
                SPAM_BLOCKED_TOTAL.labels(reason="dedup").inc()
                return None
            await r.setex(dedup_key, settings.spam_dedup_window_seconds, "1")

        return await handler(event, data)

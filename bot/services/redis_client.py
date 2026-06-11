import redis as sync_redis
import redis.asyncio as aioredis

from bot.config import settings

_async_redis: aioredis.Redis | None = None
_sync_redis: sync_redis.Redis | None = None


async def get_async_redis() -> aioredis.Redis:
    global _async_redis
    if _async_redis is None:
        _async_redis = aioredis.from_url(settings.redis_url, decode_responses=True)
    return _async_redis


def get_sync_redis() -> sync_redis.Redis:
    global _sync_redis
    if _sync_redis is None:
        _sync_redis = sync_redis.from_url(settings.redis_url, decode_responses=True)
    return _sync_redis

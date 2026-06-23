import structlog
import redis as sync_redis
import redis.asyncio as aioredis

from bot.config import settings

logger = structlog.get_logger()

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


async def close_async_redis() -> None:
    global _async_redis
    if _async_redis is not None:
        try:
            await _async_redis.aclose()
        except Exception:
            logger.warning("error closing async redis", exc_info=True)
        finally:
            _async_redis = None
            logger.info("async redis client closed")


def close_sync_redis() -> None:
    global _sync_redis
    if _sync_redis is not None:
        try:
            _sync_redis.close()
        except Exception:
            logger.warning("error closing sync redis", exc_info=True)
        finally:
            _sync_redis = None
            logger.info("sync redis client closed")

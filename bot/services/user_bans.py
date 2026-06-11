import structlog

from bot.services.redis_client import get_async_redis, get_sync_redis

logger = structlog.get_logger()

REDIS_KEY = "saveinator:banned_users"


async def ban_user(user_id: int) -> None:
    redis_client = await get_async_redis()
    await redis_client.sadd(REDIS_KEY, str(int(user_id)))
    logger.info("user banned", user_id=user_id)


async def unban_user(user_id: int) -> bool:
    redis_client = await get_async_redis()
    removed = await redis_client.srem(REDIS_KEY, str(int(user_id)))
    if removed:
        logger.info("user unbanned", user_id=user_id)
    return bool(removed)


async def is_user_banned(user_id: int) -> bool:
    redis_client = await get_async_redis()
    return bool(await redis_client.sismember(REDIS_KEY, str(int(user_id))))


async def list_banned_users() -> list[int]:
    redis_client = await get_async_redis()
    members = await redis_client.smembers(REDIS_KEY)
    return sorted(int(member) for member in members)


def is_user_banned_sync(user_id: int) -> bool:
    redis_client = get_sync_redis()
    return bool(redis_client.sismember(REDIS_KEY, str(int(user_id))))

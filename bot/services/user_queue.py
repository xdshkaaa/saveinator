import uuid
from enum import StrEnum

from bot.config import settings
from bot.services.redis_client import get_async_redis, get_sync_redis

_LOCK_PREFIX = "user_busy"
_RELEASE_SCRIPT = """
if redis.call("get", KEYS[1]) == ARGV[1] then
    return redis.call("del", KEYS[1])
end
return 0
"""


class UserScenario(StrEnum):
    VIDEO = "video"
    PINTEREST = "pinterest"
    SPOTIFY = "spotify"


def _lock_key(user_id: int) -> str:
    return f"{_LOCK_PREFIX}:{user_id}"


def _lock_value(scenario: UserScenario, token: str) -> str:
    return f"{scenario.value}:{token}"


def lock_ttl_seconds(scenario: UserScenario, track_count: int = 0) -> int:
    buffer = 30
    from bot.services.runtime_settings import (
        pinterest_timeout_seconds,
        platform_download_timeout_seconds,
        spotify_track_timeout_seconds,
    )

    if scenario == UserScenario.VIDEO:
        return max(
            platform_download_timeout_seconds("youtube"),
            platform_download_timeout_seconds("tiktok"),
            platform_download_timeout_seconds("instagram"),
            platform_download_timeout_seconds("x"),
            1,
        ) + buffer
    if scenario == UserScenario.PINTEREST:
        return max(pinterest_timeout_seconds(), 1) + buffer
    if scenario == UserScenario.SPOTIFY:
        tracks = max(track_count, 1)
        per_track = max(spotify_track_timeout_seconds(), 1)
        return tracks * per_track + buffer + 60
    return 120


async def acquire_user_lock(
    user_id: int,
    scenario: UserScenario,
    *,
    track_count: int = 0,
) -> str | None:
    token = uuid.uuid4().hex
    ttl = lock_ttl_seconds(scenario, track_count=track_count)
    redis_client = await get_async_redis()
    acquired = await redis_client.set(
        _lock_key(user_id),
        _lock_value(scenario, token),
        nx=True,
        ex=ttl,
    )
    return token if acquired else None


async def extend_user_lock(
    user_id: int,
    token: str,
    scenario: UserScenario,
    *,
    track_count: int = 0,
) -> bool:
    redis_client = await get_async_redis()
    current = await redis_client.get(_lock_key(user_id))
    if current != _lock_value(scenario, token):
        return False
    ttl = lock_ttl_seconds(scenario, track_count=track_count)
    return bool(await redis_client.expire(_lock_key(user_id), ttl))


async def release_user_lock(user_id: int, token: str, scenario: UserScenario) -> None:
    redis_client = await get_async_redis()
    await redis_client.eval(
        _RELEASE_SCRIPT,
        1,
        _lock_key(user_id),
        _lock_value(scenario, token),
    )


async def is_user_busy(user_id: int) -> bool:
    redis_client = await get_async_redis()
    return bool(await redis_client.exists(_lock_key(user_id)))


def acquire_user_lock_sync(
    user_id: int,
    scenario: UserScenario,
    *,
    track_count: int = 0,
) -> str | None:
    token = uuid.uuid4().hex
    ttl = lock_ttl_seconds(scenario, track_count=track_count)
    redis_client = get_sync_redis()
    acquired = redis_client.set(
        _lock_key(user_id),
        _lock_value(scenario, token),
        nx=True,
        ex=ttl,
    )
    return token if acquired else None


def release_user_lock_sync(user_id: int, token: str, scenario: UserScenario) -> None:
    if not token:
        return
    redis_client = get_sync_redis()
    redis_client.eval(
        _RELEASE_SCRIPT,
        1,
        _lock_key(user_id),
        _lock_value(scenario, token),
    )

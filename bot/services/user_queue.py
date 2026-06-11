import uuid
from enum import StrEnum

import redis.asyncio as aioredis
import redis as sync_redis

from bot.config import settings

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


_async_redis: aioredis.Redis | None = None
_sync_redis: sync_redis.Redis | None = None


def _lock_key(user_id: int) -> str:
    return f"{_LOCK_PREFIX}:{user_id}"


def _lock_value(scenario: UserScenario, token: str) -> str:
    return f"{scenario.value}:{token}"


def lock_ttl_seconds(scenario: UserScenario, track_count: int = 0) -> int:
    buffer = 30
    if scenario == UserScenario.VIDEO:
        return max(settings.download_timeout_seconds, 1) + buffer
    if scenario == UserScenario.PINTEREST:
        return max(settings.pinterest_timeout_seconds, 1) + buffer
    if scenario == UserScenario.SPOTIFY:
        tracks = max(track_count, 1)
        per_track = max(settings.spotify_track_timeout_seconds, 1)
        return tracks * per_track + buffer + 60
    return 120


async def _get_async_redis() -> aioredis.Redis:
    global _async_redis
    if _async_redis is None:
        _async_redis = aioredis.from_url(settings.redis_url, decode_responses=True)
    return _async_redis


def _get_sync_redis() -> sync_redis.Redis:
    global _sync_redis
    if _sync_redis is None:
        _sync_redis = sync_redis.from_url(settings.redis_url, decode_responses=True)
    return _sync_redis


async def acquire_user_lock(
    user_id: int,
    scenario: UserScenario,
    *,
    track_count: int = 0,
) -> str | None:
    token = uuid.uuid4().hex
    ttl = lock_ttl_seconds(scenario, track_count=track_count)
    redis_client = await _get_async_redis()
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
    redis_client = await _get_async_redis()
    current = await redis_client.get(_lock_key(user_id))
    if current != _lock_value(scenario, token):
        return False
    ttl = lock_ttl_seconds(scenario, track_count=track_count)
    return bool(await redis_client.expire(_lock_key(user_id), ttl))


async def release_user_lock(user_id: int, token: str, scenario: UserScenario) -> None:
    redis_client = await _get_async_redis()
    await redis_client.eval(
        _RELEASE_SCRIPT,
        1,
        _lock_key(user_id),
        _lock_value(scenario, token),
    )


def acquire_user_lock_sync(
    user_id: int,
    scenario: UserScenario,
    *,
    track_count: int = 0,
) -> str | None:
    token = uuid.uuid4().hex
    ttl = lock_ttl_seconds(scenario, track_count=track_count)
    redis_client = _get_sync_redis()
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
    redis_client = _get_sync_redis()
    redis_client.eval(
        _RELEASE_SCRIPT,
        1,
        _lock_key(user_id),
        _lock_value(scenario, token),
    )

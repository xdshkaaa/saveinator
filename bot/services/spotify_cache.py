import hashlib
import json

import structlog

from bot.services.spotify_models import NormalizedSpotifyRelease, release_from_dict, release_to_dict
from bot.services.redis_client import get_async_redis

logger = structlog.get_logger()

META_PREFIX = "spotify:meta"
YT_SEARCH_PREFIX = "yt:search"


def meta_cache_key(link_type: str, resource_id: str) -> str:
    return f"{META_PREFIX}:{link_type}:{resource_id}"


def youtube_search_cache_key(query: str) -> str:
    digest = hashlib.md5(query.encode("utf-8")).hexdigest()
    return f"{YT_SEARCH_PREFIX}:{digest}"


async def get_cached_release(
    link_type: str,
    resource_id: str,
) -> NormalizedSpotifyRelease | None:
    try:
        redis_client = await get_async_redis()
        cached = await redis_client.get(meta_cache_key(link_type, resource_id))
        if not cached:
            return None
        return release_from_dict(json.loads(cached))
    except Exception:
        logger.warning(
            "spotify metadata cache read failed",
            link_type=link_type,
            resource_id=resource_id,
            exc_info=True,
        )
        return None


async def set_cached_release(
    link_type: str,
    resource_id: str,
    release: NormalizedSpotifyRelease,
    ttl_seconds: int,
) -> None:
    try:
        redis_client = await get_async_redis()
        payload = json.dumps(release_to_dict(release))
        await redis_client.setex(meta_cache_key(link_type, resource_id), ttl_seconds, payload)
    except Exception:
        logger.warning(
            "spotify metadata cache write failed",
            link_type=link_type,
            resource_id=resource_id,
            exc_info=True,
        )


async def get_cached_youtube_video_id(query: str) -> str | None:
    try:
        redis_client = await get_async_redis()
        cached = await redis_client.get(youtube_search_cache_key(query))
        if cached:
            return cached
    except Exception:
        logger.warning("youtube search cache read failed", query=query, exc_info=True)
    return None


async def set_cached_youtube_video_id(
    query: str,
    video_id: str,
    ttl_seconds: int,
) -> None:
    if not video_id:
        return
    try:
        redis_client = await get_async_redis()
        await redis_client.setex(youtube_search_cache_key(query), ttl_seconds, video_id)
    except Exception:
        logger.warning("youtube search cache write failed", query=query, exc_info=True)

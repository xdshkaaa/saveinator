import hashlib
import json

import structlog

from bot.services.redis_client import get_async_redis
from bot.services.soundcloud_models import NormalizedSoundCloudRelease, release_from_dict, release_to_dict

logger = structlog.get_logger()

META_PREFIX = "soundcloud:meta"


def meta_cache_key(url: str) -> str:
    digest = hashlib.sha256(url.encode("utf-8")).hexdigest()
    return f"{META_PREFIX}:{digest}"


async def get_cached_release(url: str) -> NormalizedSoundCloudRelease | None:
    try:
        redis_client = await get_async_redis()
        cached = await redis_client.get(meta_cache_key(url))
        if not cached:
            return None
        return release_from_dict(json.loads(cached))
    except Exception:
        logger.warning("soundcloud metadata cache read failed", url=url, exc_info=True)
        return None


async def set_cached_release(
    url: str,
    release: NormalizedSoundCloudRelease,
    ttl_seconds: int,
) -> None:
    try:
        redis_client = await get_async_redis()
        payload = json.dumps(release_to_dict(release))
        await redis_client.setex(meta_cache_key(url), ttl_seconds, payload)
    except Exception:
        logger.warning("soundcloud metadata cache write failed", url=url, exc_info=True)

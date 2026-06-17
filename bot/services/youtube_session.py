import json
from dataclasses import asdict, dataclass
from typing import Any

from bot.services.redis_client import get_async_redis

_SESSION_PREFIX = "yt_pending"
_SESSION_TTL_SECONDS = 900


@dataclass
class YoutubePendingSession:
    user_id: int
    url: str
    chat_id: int
    message_id: int
    lang: str
    quality: int | None = None


def _session_key(user_id: int) -> str:
    return f"{_SESSION_PREFIX}:{user_id}"


def _deserialize(raw: str) -> YoutubePendingSession | None:
    try:
        data: dict[str, Any] = json.loads(raw)
        return YoutubePendingSession(
            user_id=int(data["user_id"]),
            url=str(data["url"]),
            chat_id=int(data["chat_id"]),
            message_id=int(data["message_id"]),
            lang=str(data.get("lang", "en")),
            quality=int(data["quality"]) if data.get("quality") is not None else None,
        )
    except (TypeError, ValueError, KeyError, json.JSONDecodeError):
        return None


async def save_youtube_session(session: YoutubePendingSession) -> None:
    redis_client = await get_async_redis()
    payload = json.dumps(asdict(session), ensure_ascii=False)
    await redis_client.set(_session_key(session.user_id), payload, ex=_SESSION_TTL_SECONDS)


async def get_youtube_session(user_id: int) -> YoutubePendingSession | None:
    redis_client = await get_async_redis()
    raw = await redis_client.get(_session_key(user_id))
    if not raw:
        return None
    return _deserialize(raw)


async def update_youtube_quality(user_id: int, quality: int) -> YoutubePendingSession | None:
    session = await get_youtube_session(user_id)
    if session is None:
        return None
    session.quality = quality
    await save_youtube_session(session)
    return session


async def clear_youtube_session(user_id: int) -> None:
    redis_client = await get_async_redis()
    await redis_client.delete(_session_key(user_id))

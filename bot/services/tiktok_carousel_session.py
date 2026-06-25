import json
from dataclasses import asdict, dataclass
from typing import Any

from bot.services.redis_client import get_async_redis

_SESSION_PREFIX = "ttk_carousel"
_SESSION_TTL_SECONDS = 900


@dataclass
class TikTokCarouselSession:
    user_id: int
    url: str
    chat_id: int
    lang: str
    title: str
    author: str
    token: str


def _session_key(token: str) -> str:
    return f"{_SESSION_PREFIX}:{token}"


def _deserialize(raw: str) -> TikTokCarouselSession | None:
    try:
        data: dict[str, Any] = json.loads(raw)
        return TikTokCarouselSession(
            user_id=int(data["user_id"]),
            url=str(data["url"]),
            chat_id=int(data["chat_id"]),
            lang=str(data.get("lang", "en")),
            title=str(data.get("title", "")),
            author=str(data.get("author", "")),
            token=str(data["token"]),
        )
    except (TypeError, ValueError, KeyError, json.JSONDecodeError):
        return None


async def save_tiktok_carousel_session(session: TikTokCarouselSession) -> None:
    redis_client = await get_async_redis()
    payload = json.dumps(asdict(session), ensure_ascii=False)
    await redis_client.set(
        _session_key(session.token),
        payload,
        ex=_SESSION_TTL_SECONDS,
    )


async def get_tiktok_carousel_session(token: str) -> TikTokCarouselSession | None:
    redis_client = await get_async_redis()
    raw = await redis_client.get(_session_key(token))
    if not raw:
        return None
    return _deserialize(raw)


async def delete_tiktok_carousel_session(token: str) -> None:
    redis_client = await get_async_redis()
    await redis_client.delete(_session_key(token))

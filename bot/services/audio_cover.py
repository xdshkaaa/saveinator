from pathlib import Path
from urllib.parse import urlparse

import httpx
import structlog
from aiogram.types import FSInputFile

logger = structlog.get_logger()

_SUPPORTED_IMAGE_SUFFIXES = {".jpg", ".jpeg", ".png", ".webp"}
_MAX_THUMBNAIL_BYTES = 2 * 1024 * 1024


async def fetch_audio_thumbnail(
    artwork_url: str | None,
    task_dir: Path,
    filename_prefix: str,
) -> FSInputFile | None:
    if not artwork_url:
        return None

    parsed = urlparse(artwork_url)
    if parsed.scheme not in {"http", "https"}:
        return None

    try:
        async with httpx.AsyncClient(follow_redirects=True, timeout=10) as client:
            response = await client.get(artwork_url)
            response.raise_for_status()
    except Exception:
        logger.warning("audio thumbnail download failed", artwork_url=artwork_url, exc_info=True)
        return None

    content = response.content
    if not content or len(content) > _MAX_THUMBNAIL_BYTES:
        logger.warning(
            "audio thumbnail skipped",
            artwork_url=artwork_url,
            size=len(content),
        )
        return None

    suffix = Path(parsed.path).suffix.lower()
    if suffix not in _SUPPORTED_IMAGE_SUFFIXES:
        suffix = ".jpg"

    thumbnail_path = task_dir / f"{filename_prefix}{suffix}"
    thumbnail_path.write_bytes(content)
    return FSInputFile(thumbnail_path)


async def send_audio_with_thumbnail_fallback(bot, **send_kwargs) -> None:
    try:
        await bot.send_audio(**send_kwargs)
    except Exception:
        if "thumbnail" not in send_kwargs:
            raise

        logger.warning("send_audio thumbnail rejected, retrying without thumbnail", exc_info=True)
        fallback_kwargs = dict(send_kwargs)
        fallback_kwargs.pop("thumbnail", None)
        await bot.send_audio(**fallback_kwargs)

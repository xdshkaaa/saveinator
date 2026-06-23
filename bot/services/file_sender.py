import os
from pathlib import Path

import structlog
from aiogram import Bot
from aiogram.types import FSInputFile

from bot.config import settings
from bot.locale import get
from bot.services.runtime_settings import (
    platform_max_file_mb,
    send_document_limit_mb,
    telegram_bot_upload_limit_mb,
)

logger = structlog.get_logger()

_IMAGE_EXTENSIONS = frozenset({".jpg", ".jpeg", ".png", ".webp", ".gif", ".bmp"})


def telegram_upload_limit_mb(platform: str | None = None) -> int:
    """Effective upload cap for send_file.

    Per-platform max_file_mb from admin controls both download acceptance and
    send attempts for that platform. Global telegram_bot_upload_limit_mb
    applies when no platform is specified.
    """
    limits = [send_document_limit_mb()]
    if platform is not None:
        limits.append(platform_max_file_mb(platform))
    else:
        limits.append(telegram_bot_upload_limit_mb())
    return min(limits)


def _is_image_path(file_path: Path) -> bool:
    return file_path.suffix.lower() in _IMAGE_EXTENSIONS


async def send_file(
    bot: Bot,
    file_path: Path,
    chat_id: int,
    lang: str = "en",
    title: str = "",
    bot_username: str = "saveinator_bot",
    media_type: str | None = None,
    platform: str | None = None,
) -> str:
    size_mb = os.path.getsize(file_path) / (1024 * 1024)
    caption = (
        f"{title}\n\n{get('download.via_bot', lang, bot_username=bot_username)}"
        if title
        else None
    )

    if media_type == "image" or (media_type is None and _is_image_path(file_path)):
        await bot.send_photo(
            chat_id=chat_id,
            photo=FSInputFile(file_path),
            caption=caption,
        )
        return "photo"

    if size_mb <= settings.send_video_limit_mb:
        try:
            await bot.send_video(
                chat_id=chat_id,
                video=FSInputFile(file_path),
                caption=caption,
                supports_streaming=True,
            )
            return "video"
        except Exception as exc:
            logger.warning(
                "send_video failed, falling back to document",
                chat_id=chat_id,
                size_mb=round(size_mb, 2),
                error=str(exc),
            )

    if size_mb <= telegram_upload_limit_mb(platform):
        caption = (get("download.sent_as_doc", lang, size=f"{size_mb:.1f}")
                   if size_mb > settings.send_video_limit_mb else None)
        try:
            await bot.send_document(
                chat_id=chat_id,
                document=FSInputFile(file_path),
                caption=caption,
            )
            return "document"
        except Exception as exc:
            logger.warning(
                "send_document failed",
                chat_id=chat_id,
                size_mb=round(size_mb, 2),
                error=str(exc),
            )
            if "entity too large" in str(exc).lower() or "too large" in str(exc).lower():
                return "too_large"
            raise

    return "too_large"

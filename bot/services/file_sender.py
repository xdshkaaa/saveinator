import os
from pathlib import Path

import structlog
from aiogram import Bot
from aiogram.exceptions import TelegramBadRequest
from aiogram.types import FSInputFile, InlineKeyboardMarkup, InputMediaPhoto

from bot.config import settings
from bot.locale import get
from bot.services.runtime_settings import (
    platform_max_file_mb,
    send_document_limit_mb,
    telegram_bot_upload_limit_mb,
)

logger = structlog.get_logger()

_IMAGE_EXTENSIONS = frozenset({".jpg", ".jpeg", ".png", ".webp", ".bmp"})
_GIF_EXTENSION = ".gif"
_ALBUM_CHUNK_SIZE = 10


async def _send_single_photo(
    bot: Bot,
    chat_id: int,
    image_path: Path,
    caption: str | None = None,
) -> bool:
    try:
        await bot.send_photo(
            chat_id=chat_id,
            photo=FSInputFile(image_path),
            caption=caption or None,
        )
        return True
    except Exception as exc:
        logger.warning("send_photo failed", chat_id=chat_id, error=str(exc))
        return False


async def _send_media_group(
    bot: Bot,
    chat_id: int,
    image_paths: list[Path],
    caption: str | None = None,
) -> bool:
    try:
        media = [
            InputMediaPhoto(
                media=FSInputFile(path),
                caption=caption if index == 0 else None,
            )
            for index, path in enumerate(image_paths)
        ]
        await bot.send_media_group(chat_id=chat_id, media=media)
        return True
    except TelegramBadRequest as exc:
        logger.warning(
            "send_media_group failed, falling back",
            chat_id=chat_id,
            count=len(image_paths),
            error=str(exc),
        )
        return False
    except Exception as exc:
        logger.warning(
            "send_media_group unexpected error",
            chat_id=chat_id,
            error=str(exc),
        )
        return False


async def send_photo_album(
    bot: Bot,
    chat_id: int,
    image_paths: list[Path],
    *,
    caption: str | None = None,
) -> bool:
    """Send one or more photos as a Telegram album when possible."""
    if not image_paths:
        return False

    paths = sorted(image_paths, key=lambda path: path.name)
    if len(paths) == 1:
        return await _send_single_photo(bot, chat_id, paths[0], caption)

    all_chunks_sent = True
    for chunk_index, chunk_start in enumerate(range(0, len(paths), _ALBUM_CHUNK_SIZE)):
        chunk = paths[chunk_start:chunk_start + _ALBUM_CHUNK_SIZE]
        is_first_chunk = chunk_index == 0
        chunk_caption = caption if is_first_chunk else None
        sent = await _send_media_group(bot, chat_id, chunk, chunk_caption)
        if not sent:
            for index, path in enumerate(chunk):
                path_caption = caption if (is_first_chunk and index == 0) else None
                await _send_single_photo(bot, chat_id, path, path_caption)
            all_chunks_sent = False
    return all_chunks_sent or len(paths) > 0


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


def build_media_caption(
    title: str,
    lang: str,
    *,
    bot_username: str = "saveinator_bot",
    platform: str | None = None,
) -> str | None:
    via_line = get("download.via_bot", lang, bot_username=bot_username)
    stripped = title.strip()
    if stripped:
        return f"{stripped}\n\n{via_line}"
    if platform == "tiktok":
        return via_line
    return None


async def send_file(
    bot: Bot,
    file_path: Path,
    chat_id: int,
    lang: str = "en",
    title: str = "",
    bot_username: str = "saveinator_bot",
    media_type: str | None = None,
    platform: str | None = None,
    reply_markup: InlineKeyboardMarkup | None = None,
) -> str:
    size_mb = os.path.getsize(file_path) / (1024 * 1024)
    caption = build_media_caption(
        title, lang, bot_username=bot_username, platform=platform,
    )

    is_animation = media_type == "animation" or (
        media_type is None and file_path.suffix.lower() == _GIF_EXTENSION
    )
    if is_animation:
        try:
            await bot.send_animation(
                chat_id=chat_id,
                animation=FSInputFile(file_path),
                caption=caption,
                reply_markup=reply_markup,
            )
            return "animation"
        except Exception as exc:
            logger.warning(
                "send_animation failed, falling back to document",
                chat_id=chat_id,
                size_mb=round(size_mb, 2),
                error=str(exc),
            )

    is_image = media_type == "image" or (media_type is None and _is_image_path(file_path))

    if is_image:
        try:
            await bot.send_photo(
                chat_id=chat_id,
                photo=FSInputFile(file_path),
                caption=caption,
                reply_markup=reply_markup,
            )
            return "photo"
        except Exception as exc:
            logger.warning(
                "send_photo failed, falling back to document",
                chat_id=chat_id,
                error=str(exc),
            )

    if not is_image and not is_animation and size_mb <= settings.send_video_limit_mb:
        try:
            await bot.send_video(
                chat_id=chat_id,
                video=FSInputFile(file_path),
                caption=caption,
                supports_streaming=True,
                reply_markup=reply_markup,
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

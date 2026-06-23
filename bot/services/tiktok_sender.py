import structlog
from aiogram import Bot
from aiogram.exceptions import TelegramBadRequest
from aiogram.types import FSInputFile, InputMediaPhoto
from pathlib import Path

from bot.locale import get
from bot.services.runtime_settings import get_runtime_int

logger = structlog.get_logger()

_CHUNK_SIZE = 10


async def send_carousel(
    bot: Bot,
    chat_id: int,
    image_paths: list[str | Path],
    audio_path: str | Path | None,
    lang: str,
    caption: str = "",
    *,
    status_message_id: int | None = None,
) -> bool:
    """Send a TikTok carousel (images + optional audio) to Telegram.

    Returns True if at least some media was sent successfully.
    """
    if not image_paths:
        logger.warning("no images in carousel", chat_id=chat_id)
        return False

    max_items = get_runtime_int("tiktok.carousel_max_items", default=0)
    if max_items > 0:
        image_paths = image_paths[:max_items]

    success = await _send_photos(bot, chat_id, image_paths, lang, caption)
    sent_audio = False

    if audio_path and _audio_enabled():
        try:
            audio_input = FSInputFile(str(audio_path))
            await bot.send_audio(
                chat_id=chat_id,
                audio=audio_input,
            )
            sent_audio = True
        except Exception as exc:
            logger.warning("failed to send audio", chat_id=chat_id, error=str(exc))

    if status_message_id is not None:
        try:
            await bot.delete_message(chat_id=chat_id, message_id=status_message_id)
        except Exception as exc:
            logger.warning(
                "failed to delete status message",
                chat_id=chat_id, message_id=status_message_id,
                error=str(exc),
            )

    return success or sent_audio


def _audio_enabled() -> bool:
    return get_runtime_int("tiktok.carousel_audio_enabled", default=1) == 1


async def _send_photos(
    bot: Bot,
    chat_id: int,
    image_paths: list[str | Path],
    lang: str,
    caption: str,
) -> bool:
    """Send photos in chunks, with caption only on first image of first chunk."""
    if len(image_paths) == 1:
        return await _send_single_photo(bot, chat_id, image_paths[0], caption)

    all_chunks_sent = True
    for chunk_idx, chunk_start in enumerate(range(0, len(image_paths), _CHUNK_SIZE)):
        chunk = image_paths[chunk_start:chunk_start + _CHUNK_SIZE]
        is_first_chunk = chunk_idx == 0
        chunk_caption = caption if is_first_chunk else ""
        sent = await _send_media_group(bot, chat_id, chunk, chunk_caption)
        if not sent:
            # Fallback: send individually
            for i, path in enumerate(chunk):
                path_caption = caption if (is_first_chunk and i == 0) else ""
                await _send_single_photo(bot, chat_id, path, path_caption)
            all_chunks_sent = False
    return all_chunks_sent or len(image_paths) > 0


async def _send_single_photo(
    bot: Bot,
    chat_id: int,
    image_path: str | Path,
    caption: str = "",
) -> bool:
    try:
        await bot.send_photo(
            chat_id=chat_id,
            photo=FSInputFile(str(image_path)),
            caption=caption or None,
        )
        return True
    except Exception as exc:
        logger.warning("send_photo failed", chat_id=chat_id, error=str(exc))
        return False


async def _send_media_group(
    bot: Bot,
    chat_id: int,
    image_paths: list[str | Path],
    caption: str = "",
) -> bool:
    try:
        media = [
            InputMediaPhoto(
                media=FSInputFile(str(path)),
                caption=caption if i == 0 else None,
            )
            for i, path in enumerate(image_paths)
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

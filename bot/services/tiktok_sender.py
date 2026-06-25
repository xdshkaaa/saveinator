"""Send TikTok carousel media to Telegram."""

import structlog
from aiogram import Bot
from pathlib import Path

from bot.locale import get
from bot.services.file_sender import send_photo_album
from bot.services.runtime_settings import get_runtime_int

logger = structlog.get_logger()


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

    success = await _send_photos(bot, chat_id, image_paths, caption)
    sent_audio = False

    if audio_path and _audio_enabled():
        from aiogram.types import FSInputFile

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
    caption: str,
) -> bool:
    """Send photos as a Telegram album (or single photo)."""
    paths = [Path(path) for path in image_paths]
    return await send_photo_album(bot, chat_id, paths, caption=caption or None)

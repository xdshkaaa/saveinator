import os
from pathlib import Path
from aiogram import Bot
from aiogram.types import FSInputFile

from bot.config import settings
from bot.locale import get


async def send_file(
    bot: Bot,
    file_path: Path,
    chat_id: int,
    lang: str = "en",
    title: str = "",
    bot_username: str = "saveinator_bot",
) -> str:
    size_mb = os.path.getsize(file_path) / (1024 * 1024)

    if size_mb <= settings.send_video_limit_mb:
        try:
            await bot.send_video(
                chat_id=chat_id,
                video=FSInputFile(file_path),
                caption=f"{title}\n\n{get('download.via_bot', lang, bot_username=bot_username)}" if title else None,
                supports_streaming=True,
            )
            return "video"
        except Exception:
            pass

    if size_mb <= settings.send_document_limit_mb:
        caption = (get("download.sent_as_doc", lang, size=f"{size_mb:.1f}")
                   if size_mb > settings.send_video_limit_mb else None)
        await bot.send_document(
            chat_id=chat_id,
            document=FSInputFile(file_path),
            caption=caption,
        )
        return "document"

    return "too_large"

import os
from pathlib import Path
from aiogram import Bot
from aiogram.types import FSInputFile

from bot.config import settings
from bot.locale import get

logger = __import__("structlog").get_logger()


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


async def compress_video(input_path: Path, output_path: Path, target_size_mb: float = 45) -> bool:
    import asyncio

    proc = await asyncio.create_subprocess_exec(
        "ffmpeg", "-y",
        "-i", str(input_path),
        "-c:v", "libx264",
        "-crf", "28",
        "-preset", "fast",
        "-c:a", "aac",
        "-b:a", "128k",
        "-movflags", "+faststart",
        str(output_path),
        stdout=asyncio.subprocess.DEVNULL,
        stderr=asyncio.subprocess.PIPE,
    )
    try:
        _, stderr = await asyncio.wait_for(proc.communicate(), timeout=300)
    except asyncio.TimeoutError:
        proc.kill()
        await proc.wait()
        return False

    if proc.returncode != 0:
        logger.error("ffmpeg compression failed", stderr=stderr.decode()[:500])
        return False

    return output_path.exists() and os.path.getsize(output_path) > 0

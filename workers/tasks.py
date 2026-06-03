import asyncio
import hashlib
import json
import os
import uuid
from datetime import datetime, UTC
from pathlib import Path

import redis
import structlog
from aiogram import Bot
from aiogram.types import InlineKeyboardMarkup, InlineKeyboardButton, FSInputFile
from celery import Task

from bot.config import settings
from bot.locale import get
from bot.services.formats import extract_format_options, FormatOption
from bot.services.tempfiles import tempfile_manager, sweep_stale
from bot.services.file_sender import send_file
from db.models import Download, DownloadStatus, Platform, Chat, User
from db.session import async_session_factory
from workers.app import app
from workers.downloader import fetch_info, download
from workers.transcoder import compress_video

logger = structlog.get_logger()
_redis = redis.from_url(settings.redis_url, decode_responses=True)
FORMAT_CACHE_TTL = 300


def _get_bot() -> Bot:
    return Bot(token=settings.bot_token)


def _format_size(size_bytes: int | None) -> str:
    if size_bytes is None:
        return "?"
    mb = size_bytes / 1_000_000
    if mb >= 1000:
        return f"{mb / 1000:.1f} GB"
    return f"{mb:.0f} MB"


def _quality_label(opt: FormatOption, lang: str) -> str:
    if opt.is_audio_only:
        return get("formats.audio_only", lang)
    if opt.height == 2160:
        return get("formats.quality_4k", lang)
    if opt.height:
        key = f"formats.quality_{opt.height}p"
        try:
            return get(key, lang)
        except KeyError:
            return f"🎥 {opt.height}p"
    return get("formats.quality_best", lang)


def _size_label(opt: FormatOption) -> str:
    if opt.size_bytes is None:
        return "?"
    size = _format_size(opt.size_bytes)
    if opt.size_bytes > 100_000_000:
        return size + " ⚠️"
    return size


def _build_keyboard(
    url_hash: str,
    options: list[FormatOption],
    platform: str,
    lang: str,
) -> InlineKeyboardMarkup:
    rows: list[list[InlineKeyboardButton]] = []
    for opt in options:
        label = _quality_label(opt, lang)
        size = _size_label(opt)
        cb = f"fmt|{url_hash}|{opt.format_id}|{platform}"
        rows.append([
            InlineKeyboardButton(text=f"{label}  {size}", callback_data=cb),
        ])
    rows.append([
        InlineKeyboardButton(
            text=get("formats.cancel", lang),
            callback_data=f"cancel|{url_hash}||",
        )
    ])
    return InlineKeyboardMarkup(inline_keyboard=rows)


@app.task(bind=True, max_retries=2, default_retry_delay=5)
def fetch_formats_task(
    self,
    url: str,
    url_hash: str,
    platform: str,
    chat_id: int,
    user_id: int,
    message_id: int,
    lang: str = "en",
):
    bot = _get_bot()

    async def _run():
        try:
            cache_key = f"cache:formats:{url_hash}"

            cached_raw = _redis.get(cache_key)
            if cached_raw:
                cached = json.loads(cached_raw)
                formats_data = cached["formats"]
                title = cached.get("title", "")
                duration = cached.get("duration", 0)
            else:
                info = fetch_info(url)
                formats_data = info.get("formats", [])
                title = info.get("title", "")
                duration = info.get("duration", 0)
                cache_data = {
                    "formats": formats_data,
                    "title": title,
                    "duration": duration,
                    "url": url,
                    "platform": platform,
                }
                _redis.setex(cache_key, FORMAT_CACHE_TTL, json.dumps(cache_data))

            options = extract_format_options(formats_data)

            if not options:
                await _edit_message(bot, chat_id, message_id, get("errors.no_formats", lang))
                return

            keyboard = _build_keyboard(url_hash, options, platform, lang)
            duration_str = ""
            if duration:
                mins, secs = divmod(int(duration), 60)
                duration_str = f" ({mins}:{secs:02d})"
            header = f"🎬 {title}{duration_str}"

            await bot.edit_message_text(
                chat_id=chat_id,
                message_id=message_id,
                text=header,
                reply_markup=keyboard,
            )

        except Exception as exc:
            logger.exception("fetch_formats_task failed", url=url)
            await _edit_message(
                bot, chat_id, message_id,
                get("download.failed", lang, reason=str(exc)[:200]),
            )
            raise self.retry(exc=exc)

    asyncio.run(_run())


@app.task(bind=True, max_retries=1)
def download_and_send_task(
    self,
    url_hash: str,
    format_id: str,
    platform: str,
    chat_id: int,
    user_id: int,
    message_id: int,
    lang: str = "en",
):
    bot = _get_bot()
    task_id = str(uuid.uuid4())

    async def _run():
        with tempfile_manager(task_id) as task_dir:
            try:
                cache_key = f"cache:formats:{url_hash}"
                cached_raw = _redis.get(cache_key)
                if not cached_raw:
                    await _edit_message(bot, chat_id, message_id, get("errors.generic", lang))
                    return

                cached = json.loads(cached_raw)
                url = cached.get("url", "")
                title = cached.get("title", "video")

                if not url:
                    await _edit_message(bot, chat_id, message_id, get("errors.generic", lang))
                    return

                await _edit_message(bot, chat_id, message_id, get("download.downloading", lang))

                info = download(url, task_dir, format_id)

                downloaded_path: Path | None = None
                for f in task_dir.iterdir():
                    if f.is_file():
                        downloaded_path = f
                        break

                if not downloaded_path:
                    await _edit_message(
                        bot, chat_id, message_id,
                        get("download.failed", lang, reason="No output file"),
                    )
                    return

                size_mb = os.path.getsize(downloaded_path) / (1024 * 1024)

                if size_mb <= settings.send_video_limit_mb:
                    await send_file(bot, downloaded_path, chat_id, lang, title)
                    await _edit_message(
                        bot, chat_id, message_id,
                        get("download.complete", lang, title=title),
                    )
                    await _record_download(
                        url, platform, format_id, size_mb,
                        DownloadStatus.COMPLETED, user_id, chat_id,
                    )
                    return

                if size_mb <= settings.send_document_limit_mb:
                    await bot.send_document(
                        chat_id=chat_id,
                        document=FSInputFile(downloaded_path),
                        caption=get("download.sent_as_doc", lang, size=f"{size_mb:.1f}"),
                    )
                    await _edit_message(
                        bot, chat_id, message_id,
                        get("download.complete", lang, title=title),
                    )
                    await _record_download(
                        url, platform, format_id, size_mb,
                        DownloadStatus.COMPLETED, user_id, chat_id,
                    )
                    return

                await _edit_message(
                    bot, chat_id, message_id,
                    get("download.transcoding", lang),
                )

                compressed_path = task_dir / f"compressed_{task_id}.mp4"
                ok = await compress_video(downloaded_path, compressed_path)

                if ok and compressed_path.exists():
                    compressed_mb = os.path.getsize(compressed_path) / (1024 * 1024)
                    final_path = compressed_path
                    final_mb = compressed_mb

                    if compressed_mb <= settings.send_video_limit_mb:
                        await send_file(bot, final_path, chat_id, lang, title)
                        await _edit_message(
                            bot, chat_id, message_id,
                            get("download.complete", lang, title=title),
                        )
                        await _record_download(
                            url, platform, format_id, compressed_mb,
                            DownloadStatus.COMPLETED, user_id, chat_id,
                        )
                        return

                    if compressed_mb <= settings.send_document_limit_mb:
                        await bot.send_document(
                            chat_id=chat_id,
                            document=FSInputFile(final_path),
                        )
                        await _edit_message(
                            bot, chat_id, message_id,
                            get("download.complete", lang, title=title),
                        )
                        await _record_download(
                            url, platform, format_id, compressed_mb,
                            DownloadStatus.COMPLETED, user_id, chat_id,
                        )
                        return

                await _edit_message(
                    bot, chat_id, message_id,
                    get("download.too_large", lang, size=f"{size_mb:.1f}"),
                )
                await _record_download(
                    url, platform, format_id, size_mb,
                    DownloadStatus.COMPLETED, user_id, chat_id,
                )

            except Exception as exc:
                logger.exception("download_and_send_task failed", task_id=task_id)
                try:
                    await _edit_message(
                        bot, chat_id, message_id,
                        get("download.failed", lang, reason=str(exc)[:200]),
                    )
                except Exception:
                    pass
                await _record_download(
                    "", platform, format_id, 0,
                    DownloadStatus.FAILED, user_id, chat_id,
                    str(exc)[:500],
                )

    asyncio.run(_run())


@app.task
def cleanup_stale_task():
    sweep_stale()


async def _edit_message(bot: Bot, chat_id: int, message_id: int, text: str):
    try:
        await bot.edit_message_text(chat_id=chat_id, message_id=message_id, text=text)
    except Exception:
        pass


async def _record_download(
    url: str,
    platform: str,
    format_id: str,
    size_mb: float,
    status: DownloadStatus,
    user_id: int,
    chat_id: int,
    error: str | None = None,
):
    async with async_session_factory() as session:
        user = await session.get(User, user_id)
        if not user:
            user = User(id=user_id, username="unknown", language="en")
            session.add(user)

        chat = await session.get(Chat, chat_id)
        if not chat:
            chat = Chat(id=chat_id, title="unknown", type="group")
            session.add(chat)

        dl = Download(
            user_id=user_id,
            chat_id=chat_id,
            url=url,
            platform=Platform(platform) if platform else Platform.UNKNOWN,
            format_id=format_id,
            file_size=int(size_mb * 1024 * 1024) if size_mb else None,
            status=status,
            error_message=error,
            completed_at=datetime.now(UTC) if status in (DownloadStatus.COMPLETED, DownloadStatus.FAILED) else None,
        )
        session.add(dl)
        await session.commit()

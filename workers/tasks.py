import asyncio
import os
import signal
import uuid
from pathlib import Path

import structlog
from aiogram import Bot

from bot.config import settings
from bot.locale import get
from bot.services import runtime_settings
from bot.services.tempfiles import tempfile_manager, sweep_stale
from bot.services.file_sender import send_file
from db.models import Download, DownloadStatus, Platform, Chat, User, utc_now_naive
from db.session import async_session_factory
from bot.services.user_queue import UserScenario, release_user_lock_sync
from workers.app import app
from workers.downloader import download
from workers.metrics import YTDLP_ERRORS_TOTAL
from workers.video_processor import VideoProcessingError, apply_aspect_ratio
from workers.youtube_format import build_youtube_format

logger = structlog.get_logger()


class DownloadTimeoutError(Exception):
    pass


def _get_bot() -> Bot:
    return Bot(token=settings.bot_token)


def _raise_download_timeout(_signum, _frame):
    raise DownloadTimeoutError


def _platform_max_file_mb(platform: str) -> int:
    return runtime_settings.platform_max_file_mb(platform)


def _platform_download_timeout_seconds(platform: str) -> int:
    return runtime_settings.platform_download_timeout_seconds(platform)


def _download_with_timeout(url: str, output_dir: Path, format_id: str, platform: str) -> dict:
    timeout = _platform_download_timeout_seconds(platform)
    if timeout <= 0:
        return download(url, output_dir, format_id)

    previous_handler = signal.getsignal(signal.SIGALRM)
    signal.signal(signal.SIGALRM, _raise_download_timeout)
    signal.setitimer(signal.ITIMER_REAL, timeout)
    try:
        return download(url, output_dir, format_id)
    finally:
        signal.setitimer(signal.ITIMER_REAL, 0)
        signal.signal(signal.SIGALRM, previous_handler)


def _resolve_format_id(
    platform: str,
    format_id: str,
    quality: int | None,
    aspect_ratio: str | None,
) -> str:
    if platform == "youtube" and quality is not None:
        ytdlp_format = build_youtube_format(quality)
        if aspect_ratio:
            return f"{ytdlp_format}|q{quality}|r{aspect_ratio}"
        return f"{ytdlp_format}|q{quality}"
    return format_id


def _youtube_error_message(lang: str, platform: str) -> str:
    if platform == "youtube":
        return get("youtube.process_failed", lang)
    return get("errors.generic", lang)


@app.task(bind=True, max_retries=1)
def download_and_send_task(
    self,
    url: str,
    platform: str,
    chat_id: int,
    user_id: int,
    message_id: int,
    lang: str = "en",
    format_id: str = "best",
    lock_token: str = "",
    quality: int | None = None,
    aspect_ratio: str | None = None,
):
    bot = _get_bot()
    task_id = str(uuid.uuid4())
    resolved_format_id = _resolve_format_id(platform, format_id, quality, aspect_ratio)
    ytdlp_format = (
        build_youtube_format(quality)
        if platform == "youtube" and quality is not None
        else format_id
    )

    async def _run():
        with tempfile_manager(task_id) as task_dir:
            try:
                if not (platform == "youtube" and quality is not None and aspect_ratio):
                    await _edit_message(bot, chat_id, message_id, get("download.downloading", lang))

                info = _download_with_timeout(url, task_dir, ytdlp_format, platform)
                title = info.get("title") or "video"

                downloaded_path: Path | None = None
                for f in task_dir.iterdir():
                    if f.is_file():
                        downloaded_path = f
                        break

                if not downloaded_path:
                    await _edit_message(
                        bot, chat_id, message_id, _youtube_error_message(lang, platform),
                    )
                    return

                if platform == "youtube" and quality is not None and aspect_ratio:
                    try:
                        processed_path = apply_aspect_ratio(downloaded_path, aspect_ratio, quality)
                        if processed_path != downloaded_path:
                            downloaded_path.unlink(missing_ok=True)
                        downloaded_path = processed_path
                    except VideoProcessingError as exc:
                        YTDLP_ERRORS_TOTAL.labels(platform=platform).inc()
                        logger.warning(
                            "youtube video processing failed",
                            task_id=task_id,
                            url=url,
                            detail=str(exc),
                        )
                        await _edit_message(
                            bot, chat_id, message_id, _youtube_error_message(lang, platform),
                        )
                        await _record_download_safe(
                            url, platform, resolved_format_id, 0,
                            DownloadStatus.FAILED, user_id, chat_id,
                            str(exc)[:500],
                        )
                        return

                size_mb = os.path.getsize(downloaded_path) / (1024 * 1024)

                max_file_mb = _platform_max_file_mb(platform)
                if size_mb <= max_file_mb:
                    await _delete_message(bot, chat_id, message_id)
                    await send_file(bot, downloaded_path, chat_id, lang, title)
                    await _record_download_safe(
                        url, platform, resolved_format_id, size_mb,
                        DownloadStatus.COMPLETED, user_id, chat_id,
                    )
                    return

                await _edit_message(
                    bot, chat_id, message_id,
                    get(
                        "download.too_large",
                        lang,
                        size=f"{size_mb:.1f}",
                        limit=max_file_mb,
                    ),
                )
                await _record_download_safe(
                    url, platform, resolved_format_id, size_mb,
                    DownloadStatus.FAILED, user_id, chat_id,
                    f"file is larger than {max_file_mb} MB",
                )

            except DownloadTimeoutError:
                YTDLP_ERRORS_TOTAL.labels(platform=platform).inc()
                logger.warning("download timed out", task_id=task_id, url=url)
                await _edit_message(bot, chat_id, message_id, get("download.timeout", lang))
                await _record_download_safe(
                    url, platform, resolved_format_id, 0,
                    DownloadStatus.FAILED, user_id, chat_id,
                    f"download exceeded {_platform_download_timeout_seconds(platform)} seconds",
                )

            except Exception as exc:
                YTDLP_ERRORS_TOTAL.labels(platform=platform).inc()
                logger.exception("download_and_send_task failed", task_id=task_id)
                try:
                    await _edit_message(
                        bot, chat_id, message_id, _youtube_error_message(lang, platform),
                    )
                except Exception:
                    pass
                try:
                    await _record_download(
                        url, platform, resolved_format_id, 0,
                        DownloadStatus.FAILED, user_id, chat_id,
                        str(exc)[:500],
                    )
                except Exception:
                    logger.exception("failed to record failed download", task_id=task_id)
            finally:
                release_user_lock_sync(user_id, lock_token, UserScenario.VIDEO)

    asyncio.run(_run())


@app.task
def cleanup_stale_task():
    sweep_stale()


async def _edit_message(bot: Bot, chat_id: int, message_id: int, text: str):
    try:
        await bot.edit_message_text(chat_id=chat_id, message_id=message_id, text=text)
    except Exception:
        pass


async def _delete_message(bot: Bot, chat_id: int, message_id: int):
    try:
        await bot.delete_message(chat_id=chat_id, message_id=message_id)
    except Exception:
        pass


async def _record_download_safe(
    url: str,
    platform: str,
    format_id: str,
    size_mb: float,
    status: DownloadStatus,
    user_id: int,
    chat_id: int,
    error: str | None = None,
):
    try:
        await _record_download(url, platform, format_id, size_mb, status, user_id, chat_id, error)
    except Exception:
        logger.exception("failed to record download history", url=url, platform=platform, status=status.value)


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
            completed_at=utc_now_naive() if status in (DownloadStatus.COMPLETED, DownloadStatus.FAILED) else None,
        )
        session.add(dl)
        await session.commit()

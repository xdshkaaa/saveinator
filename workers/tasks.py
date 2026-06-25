import asyncio
import os
import subprocess
import uuid
from pathlib import Path

import structlog

from bot.config import settings
from bot.locale import get
from bot.metrics import record_user_created
from bot.services.runtime_settings import platform_max_file_mb, platform_download_timeout_seconds, send_document_limit_mb
from bot.services.tempfiles import tempfile_manager, sweep_stale
from bot.services.file_sender import send_file, telegram_upload_limit_mb
from db.models import Download, DownloadStatus, Platform, Chat, User, utc_now_naive
from db.session import async_session_factory
from bot.services.user_queue import UserScenario, release_user_lock_sync
from workers.app import app
from workers.bot import get_bot
from workers.downloader import (
    download,
    download_with_reply_filter,
    XTargetReplyNotFoundError,
    XTargetReplyNoMediaError,
)
from workers.metrics import DOWNLOAD_FILE_SIZE_BYTES, YTDLP_ERRORS_TOTAL
from workers.video_processor import VideoProcessingError, apply_aspect_ratio
from workers.youtube_format import build_youtube_format

logger = structlog.get_logger()

_VIDEO_EXTENSIONS = frozenset({".mp4", ".webm", ".mkv", ".mov", ".m4v"})
_IMAGE_EXTENSIONS = frozenset({".jpg", ".jpeg", ".png", ".webp"})


def _platform_max_file_mb(platform: str) -> int:
    return platform_max_file_mb(platform)


def _platform_download_timeout_seconds(platform: str) -> int:
    return platform_download_timeout_seconds(platform)


async def _download_with_timeout(
    url: str,
    output_dir: Path,
    format_id: str,
    platform: str,
    x_status_id: str | None = None,
) -> dict:
    """Run yt-dlp download in a thread with an async timeout."""
    timeout = _platform_download_timeout_seconds(platform)
    if x_status_id:
        coro = asyncio.to_thread(download_with_reply_filter, url, output_dir, format_id, x_status_id)
    else:
        coro = asyncio.to_thread(download, url, output_dir, format_id)
    if timeout <= 0:
        return await coro
    return await asyncio.wait_for(coro, timeout=timeout)


async def _download_and_process_youtube(
    url: str,
    task_dir: Path,
    ytdlp_format: str,
    aspect_ratio: str,
    quality: int,
) -> tuple[dict, Path]:
    """Download and transcode a YouTube video within a single timeout."""
    timeout = _platform_download_timeout_seconds("youtube")

    def _run() -> tuple[dict, Path]:
        info = download(url, task_dir, ytdlp_format)
        downloaded_path = _find_downloaded_video(task_dir)
        if not downloaded_path:
            raise VideoProcessingError("no video file after download")
        processed_path = apply_aspect_ratio(downloaded_path, aspect_ratio, quality)
        if processed_path != downloaded_path:
            downloaded_path.unlink(missing_ok=True)
        return info, processed_path

    if timeout <= 0:
        return await asyncio.to_thread(_run)
    return await asyncio.wait_for(asyncio.to_thread(_run), timeout=timeout)


def _resolve_format_id(
    platform: str,
    format_id: str,
    quality: int | None,
    aspect_ratio: str | None,
) -> str:
    if platform == "youtube" and quality is not None:
        if aspect_ratio:
            return f"q{quality}|r{aspect_ratio}"
        return f"q{quality}"
    return format_id


def _youtube_error_message(lang: str, platform: str) -> str:
    if platform == "youtube":
        return get("youtube.process_failed", lang)
    return get("errors.generic", lang)


def _find_downloaded_video(task_dir: Path) -> Path | None:
    candidates = [
        path
        for path in task_dir.iterdir()
        if path.is_file() and path.suffix.lower() in _VIDEO_EXTENSIONS
    ]
    if not candidates:
        return None
    return max(candidates, key=lambda path: path.stat().st_size)


def _find_downloaded_media(task_dir: Path) -> list[Path]:
    """Find all downloaded media files (images and videos) in task_dir."""
    return [
        path
        for path in task_dir.iterdir()
        if path.is_file() and path.suffix.lower() in (_VIDEO_EXTENSIONS | _IMAGE_EXTENSIONS)
    ]


def _has_audio_stream(file_path: Path) -> bool:
    """Check if a media file has an audio stream using ffprobe.

    Used to distinguish animated GIF-variants (no audio) from regular
    videos on platforms like X/Twitter where both are delivered as .mp4.
    """
    try:
        result = subprocess.run(
            [
                "ffprobe", "-v", "error",
                "-select_streams", "a",
                "-show_entries", "stream=index",
                "-of", "csv=p=0",
                str(file_path),
            ],
            capture_output=True, text=True, timeout=15,
        )
        return bool(result.stdout.strip())
    except Exception:
        logger.warning("ffprobe check failed", path=str(file_path), exc_info=True)
        return True  # assume audio present on error (safe default)


async def _run_download_and_send(
    bot,
    task_id: str,
    url: str,
    platform: str,
    chat_id: int,
    user_id: int,
    message_id: int,
    lang: str,
    resolved_format_id: str,
    ytdlp_format: str,
    lock_token: str,
    quality: int | None,
    aspect_ratio: str | None,
    x_status_id: str | None = None,
) -> None:
    """Async core of download_and_send_task.

    Extracted to module level so the Celery task function is a thin
    ``asyncio.run()`` wrapper, keeping event-loop creation explicit.
    """
    with tempfile_manager(task_id) as task_dir:
        try:
            if not (platform == "youtube" and quality is not None and aspect_ratio):
                await _edit_message(bot, chat_id, message_id, get("download.downloading", lang))

            if platform == "youtube" and quality is not None and aspect_ratio:
                try:
                    info, downloaded_path = await _download_and_process_youtube(
                        url, task_dir, ytdlp_format, aspect_ratio, quality,
                    )
                except asyncio.TimeoutError:
                    YTDLP_ERRORS_TOTAL.labels(platform=platform).inc()
                    logger.warning(
                        "youtube processing timed out",
                        task_id=task_id, url=url,
                        timeout=_platform_download_timeout_seconds(platform),
                    )
                    await _edit_message(bot, chat_id, message_id, get("download.timeout", lang))
                    await _record_download_safe(
                        url, platform, resolved_format_id, 0,
                        DownloadStatus.FAILED, user_id, chat_id,
                        f"processing exceeded {_platform_download_timeout_seconds(platform)} seconds",
                    )
                    return
                except VideoProcessingError as exc:
                    YTDLP_ERRORS_TOTAL.labels(platform=platform).inc()
                    logger.warning(
                        "youtube video processing failed",
                        task_id=task_id, url=url, detail=str(exc),
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
                title = info.get("title") or "video"
            else:
                try:
                    info = await _download_with_timeout(
                        url, task_dir, ytdlp_format, platform,
                        x_status_id=x_status_id,
                    )
                except (XTargetReplyNotFoundError, XTargetReplyNoMediaError):
                    await _edit_message(
                        bot, chat_id, message_id,
                        get("x.no_media_in_reply", lang),
                    )
                    await _record_download_safe(
                        url, platform, resolved_format_id, 0,
                        DownloadStatus.FAILED, user_id, chat_id,
                        "target reply has no downloadable media",
                    )
                    return
                except asyncio.TimeoutError:
                    YTDLP_ERRORS_TOTAL.labels(platform=platform).inc()
                    logger.warning(
                        "download timed out",
                        task_id=task_id, url=url,
                        timeout=_platform_download_timeout_seconds(platform),
                    )
                    await _edit_message(bot, chat_id, message_id, get("download.timeout", lang))
                    await _record_download_safe(
                        url, platform, resolved_format_id, 0,
                        DownloadStatus.FAILED, user_id, chat_id,
                        f"download exceeded {_platform_download_timeout_seconds(platform)} seconds",
                    )
                    return
                media_files = _find_downloaded_media(task_dir)
                video_files = [
                    f for f in media_files
                    if f.suffix.lower() in _VIDEO_EXTENSIONS
                ]
                image_files = [
                    f for f in media_files
                    if f.suffix.lower() in _IMAGE_EXTENSIONS
                ]
                title = info.get("title") or "video"

                # --- Image-only path (generic platforms only) ---
                if not video_files and image_files:
                    total_bytes = sum(os.path.getsize(p) for p in image_files)
                    for img_path in image_files:
                        await send_file(
                            bot, img_path, chat_id, lang, title,
                            platform=platform, media_type="image",
                        )
                    await _delete_message(bot, chat_id, message_id)
                    await _record_download_safe(
                        url, platform, resolved_format_id,
                        total_bytes / (1024 * 1024),
                        DownloadStatus.COMPLETED, user_id, chat_id,
                    )
                    DOWNLOAD_FILE_SIZE_BYTES.labels(platform=platform).observe(total_bytes)
                    return

                # --- Generic video path ---
                downloaded_path = max(video_files, key=lambda p: p.stat().st_size) if video_files else None

            if not downloaded_path:
                await _edit_message(
                    bot, chat_id, message_id, _youtube_error_message(lang, platform),
                )
                return

            size_mb = os.path.getsize(downloaded_path) / (1024 * 1024)

            max_file_mb = _platform_max_file_mb(platform)
            if size_mb <= max_file_mb:
                # Detect X/Twitter GIFs (delivered as silent mp4) so they're sent
                # as Telegram animations rather than regular videos.
                media_type = "animation" if (
                    platform == "x"
                    and not _has_audio_stream(downloaded_path)
                ) else None
                send_result = await send_file(
                    bot, downloaded_path, chat_id, lang, title,
                    media_type=media_type, platform=platform,
                )
                if send_result == "too_large":
                    doc_limit = telegram_upload_limit_mb(platform)
                    await _edit_message(
                        bot, chat_id, message_id,
                        get(
                            "download.too_large",
                            lang,
                            size=f"{size_mb:.1f}",
                            limit=doc_limit,
                        ),
                    )
                    await _record_download_safe(
                        url, platform, resolved_format_id, size_mb,
                        DownloadStatus.FAILED, user_id, chat_id,
                        f"file exceeds Telegram send limit of {doc_limit} MB",
                    )
                    return

                await _delete_message(bot, chat_id, message_id)
                await _record_download_safe(
                    url, platform, resolved_format_id, size_mb,
                    DownloadStatus.COMPLETED, user_id, chat_id,
                )
                DOWNLOAD_FILE_SIZE_BYTES.labels(platform=platform).observe(
                    size_mb * 1024 * 1024
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

        except Exception as exc:
            YTDLP_ERRORS_TOTAL.labels(platform=platform).inc()
            logger.exception("download_and_send_task failed", task_id=task_id)
            try:
                await _edit_message(
                    bot, chat_id, message_id, _youtube_error_message(lang, platform),
                )
            except Exception as edit_err:
                logger.warning(
                    "failed to edit error message",
                    chat_id=chat_id, message_id=message_id,
                    error=str(edit_err),
                )
            try:
                await _record_download(
                    url, platform, resolved_format_id, 0,
                    DownloadStatus.FAILED, user_id, chat_id,
                    str(exc)[:500],
                )
            except Exception as rec_err:
                logger.exception("failed to record failed download", task_id=task_id, error=str(rec_err))
        finally:
            release_user_lock_sync(user_id, lock_token, UserScenario.VIDEO)


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
    x_status_id: str | None = None,
):
    bot = get_bot()
    task_id = str(uuid.uuid4())
    resolved_format_id = _resolve_format_id(platform, format_id, quality, aspect_ratio)
    ytdlp_format = (
        build_youtube_format(quality, aspect_ratio)
        if platform == "youtube" and quality is not None
        else format_id
    )

    # Celery tasks are synchronous; asyncio.run() is the standard bridge
    # to run the async core once per task.
    asyncio.run(
        _run_download_and_send(
            bot=bot,
            task_id=task_id,
            url=url,
            platform=platform,
            chat_id=chat_id,
            user_id=user_id,
            message_id=message_id,
            lang=lang,
            resolved_format_id=resolved_format_id,
            ytdlp_format=ytdlp_format,
            lock_token=lock_token,
            quality=quality,
            aspect_ratio=aspect_ratio,
            x_status_id=x_status_id,
        )
    )


@app.task
def cleanup_stale_task():
    sweep_stale()


async def _edit_message(bot, chat_id: int, message_id: int, text: str):
    try:
        await bot.edit_message_text(chat_id=chat_id, message_id=message_id, text=text)
    except Exception as exc:
        logger.warning(
            "edit_message failed",
            chat_id=chat_id, message_id=message_id,
            error=str(exc),
            action="edit_message",
        )


async def _delete_message(bot, chat_id: int, message_id: int):
    try:
        await bot.delete_message(chat_id=chat_id, message_id=message_id)
    except Exception as exc:
        logger.warning(
            "delete_message failed",
            chat_id=chat_id, message_id=message_id,
            error=str(exc),
            action="delete_message",
        )


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
            record_user_created()

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

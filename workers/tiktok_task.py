import asyncio
import uuid
from pathlib import Path

import structlog

from bot.config import settings
from bot.locale import get
from bot.metrics import record_user_created
from bot.services.runtime_settings import platform_max_file_mb, platform_download_timeout_seconds
from bot.services.tempfiles import tempfile_manager
from bot.services.file_sender import send_file, telegram_upload_limit_mb
from bot.services.tiktok_sender import send_carousel
from db.models import Download, DownloadStatus, Platform, Chat, User, utc_now_naive
from db.session import async_session_factory
from bot.services.user_queue import UserScenario, release_user_lock_sync
from workers.app import app
from workers.bot import get_bot
from workers.tiktok_downloader import (
    TikTokDownloadError,
    TikTokNoMediaError,
    TikTokPostType,
    download_tiktok,
)
from workers.metrics import YTDLP_ERRORS_TOTAL, DOWNLOAD_FILE_SIZE_BYTES

logger = structlog.get_logger()


async def _run_tiktok_download(
    bot,
    task_id: str,
    url: str,
    chat_id: int,
    user_id: int,
    message_id: int,
    lang: str,
    lock_token: str,
) -> None:
    """Async core of tiktok_download_task."""
    with tempfile_manager(task_id) as task_dir:
        try:
            await _edit_message(
                bot, chat_id, message_id, get("download.downloading", lang)
            )

            timeout = platform_download_timeout_seconds("tiktok")
            try:
                if timeout > 0:
                    result = await asyncio.wait_for(
                        asyncio.to_thread(download_tiktok, url, task_dir),
                        timeout=timeout,
                    )
                else:
                    result = await asyncio.to_thread(download_tiktok, url, task_dir)
            except asyncio.TimeoutError:
                YTDLP_ERRORS_TOTAL.labels(platform="tiktok").inc()
                logger.warning(
                    "tiktok download timed out",
                    task_id=task_id, url=url,
                    timeout=timeout,
                )
                await _edit_message(
                    bot, chat_id, message_id, get("download.timeout", lang)
                )
                await _record_download_safe(
                    url, "tiktok", "best", 0,
                    DownloadStatus.FAILED, user_id, chat_id,
                    f"exceeded {timeout}s timeout",
                )
                return

            if result.errors and not result.images and not result.video_path:
                await _edit_message(
                    bot, chat_id, message_id,
                    result.errors[0] or get("errors.generic", lang),
                )
                await _record_download_safe(
                    url, "tiktok", "best", 0,
                    DownloadStatus.FAILED, user_id, chat_id,
                    "; ".join(result.errors),
                )
                return

            await _delete_message(bot, chat_id, message_id)
            max_file_mb = platform_max_file_mb("tiktok")

            if result.post_type in (TikTokPostType.CAROUSEL, TikTokPostType.AUDIO_ONLY):
                # Carousel flow: send images + audio
                caption_parts = []
                if result.title:
                    caption_parts.append(result.title)
                if result.author:
                    caption_parts.append(f"@{result.author}")
                caption = "\n".join(caption_parts) if caption_parts else ""

                await send_carousel(
                    bot, chat_id,
                    result.images, result.audio,
                    lang, caption=caption,
                )

                await _record_download_safe(
                    url, "tiktok", "carousel",
                    _total_image_size_mb(result.images),
                    DownloadStatus.COMPLETED, user_id, chat_id,
                )

            elif result.post_type == TikTokPostType.VIDEO:
                if not result.video_path:
                    await _edit_message(
                        bot, chat_id, message_id, get("errors.generic", lang),
                    )
                    return

                video_path = Path(result.video_path)
                size_mb = video_path.stat().st_size / (1024 * 1024)
                title = result.title or "video"

                if size_mb <= max_file_mb:
                    send_result = await send_file(
                        bot, video_path, chat_id, lang, title,
                        platform="tiktok",
                    )
                    if send_result == "too_large":
                        doc_limit = telegram_upload_limit_mb("tiktok")
                        await bot.send_message(
                            chat_id,
                            get(
                                "download.too_large", lang,
                                size=f"{size_mb:.1f}", limit=doc_limit,
                            ),
                        )
                        await _record_download_safe(
                            url, "tiktok", "best", size_mb,
                            DownloadStatus.FAILED, user_id, chat_id,
                            f"file exceeds Telegram limit of {doc_limit} MB",
                        )
                        return

                    await _record_download_safe(
                        url, "tiktok", "best", size_mb,
                        DownloadStatus.COMPLETED, user_id, chat_id,
                    )
                    DOWNLOAD_FILE_SIZE_BYTES.labels(platform="tiktok").observe(
                        size_mb * 1024 * 1024,
                    )
                    return

                await bot.send_message(
                    chat_id,
                    get(
                        "download.too_large", lang,
                        size=f"{size_mb:.1f}", limit=max_file_mb,
                    ),
                )
                await _record_download_safe(
                    url, "tiktok", "best", size_mb,
                    DownloadStatus.FAILED, user_id, chat_id,
                    f"file is larger than {max_file_mb} MB",
                )

            else:
                await _edit_message(
                    bot, chat_id, message_id, get("errors.generic", lang),
                )

        except TikTokNoMediaError:
            await _edit_message(
                bot, chat_id, message_id, get("errors.generic", lang),
            )
            await _record_download_safe(
                url, "tiktok", "best", 0,
                DownloadStatus.FAILED, user_id, chat_id,
                "no media found",
            )

        except TikTokDownloadError as exc:
            YTDLP_ERRORS_TOTAL.labels(platform="tiktok").inc()
            logger.warning(
                "tiktok download failed",
                task_id=task_id, error=str(exc),
            )
            await _edit_message(
                bot, chat_id, message_id, get("errors.generic", lang),
            )
            await _record_download_safe(
                url, "tiktok", "best", 0,
                DownloadStatus.FAILED, user_id, chat_id,
                str(exc)[:500],
            )

        except Exception as exc:
            YTDLP_ERRORS_TOTAL.labels(platform="tiktok").inc()
            logger.exception("tiktok_download_task failed", task_id=task_id)
            try:
                await _edit_message(
                    bot, chat_id, message_id, get("errors.generic", lang),
                )
            except Exception as edit_err:
                logger.warning(
                    "failed to edit error message",
                    chat_id=chat_id, message_id=message_id,
                    error=str(edit_err),
                )
            try:
                await _record_download(
                    url, "tiktok", "best", 0,
                    DownloadStatus.FAILED, user_id, chat_id,
                    str(exc)[:500],
                )
            except Exception as rec_err:
                logger.exception(
                    "failed to record failed download",
                    task_id=task_id, error=str(rec_err),
                )
        finally:
            release_user_lock_sync(user_id, lock_token, UserScenario.TIKTOK)


@app.task(bind=True, max_retries=1)
def tiktok_download_task(
    self,
    url: str,
    chat_id: int,
    user_id: int,
    message_id: int,
    lang: str = "en",
    lock_token: str = "",
):
    bot = get_bot()
    task_id = str(uuid.uuid4())

    asyncio.run(
        _run_tiktok_download(
            bot=bot,
            task_id=task_id,
            url=url,
            chat_id=chat_id,
            user_id=user_id,
            message_id=message_id,
            lang=lang,
            lock_token=lock_token,
        )
    )


def _total_image_size_mb(image_paths: list[str]) -> float:
    total = 0.0
    for path in image_paths:
        try:
            total += Path(path).stat().st_size
        except OSError:
            pass
    return total / (1024 * 1024)


async def _edit_message(bot, chat_id: int, message_id: int, text: str):
    try:
        await bot.edit_message_text(
            chat_id=chat_id, message_id=message_id, text=text,
        )
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
        await _record_download(
            url, platform, format_id, size_mb,
            status, user_id, chat_id, error,
        )
    except Exception:
        logger.exception(
            "failed to record download history",
            url=url, platform=platform, status=status.value,
        )


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
            completed_at=utc_now_naive()
            if status in (DownloadStatus.COMPLETED, DownloadStatus.FAILED)
            else None,
        )
        session.add(dl)
        await session.commit()
        if status == DownloadStatus.COMPLETED and size_mb:
            DOWNLOAD_FILE_SIZE_BYTES.labels(platform=platform).observe(
                size_mb * 1024 * 1024,
            )

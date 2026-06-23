import asyncio
import os
import uuid
from pathlib import Path

import structlog

from bot.config import settings
from bot.locale import get
from bot.metrics import record_user_created
from bot.services.runtime_settings import (
    pinterest_max_file_mb,
    pinterest_timeout_seconds,
    send_document_limit_mb,
)
from bot.services.file_sender import send_file
from bot.services.tempfiles import tempfile_manager
from db.models import Download, DownloadStatus, Platform, Chat, User, utc_now_naive
from db.session import async_session_factory
from bot.services.user_queue import UserScenario, release_user_lock_sync
from workers.app import app
from workers.bot import get_bot
from workers.pinterest_downloader import (
    PinterestDownloadError,
    PinterestNoMediaError,
    download_pinterest,
)
from workers.metrics import DOWNLOAD_FILE_SIZE_BYTES

logger = structlog.get_logger()


async def _download_pinterest_with_timeout(
    url: str,
    task_dir: Path,
    max_items: int,
    timeout: int,
):
    """Run Pinterest download in a thread with an async timeout."""
    coro = asyncio.to_thread(download_pinterest, url, task_dir, max_items=max_items)
    if timeout <= 0:
        return await coro
    return await asyncio.wait_for(coro, timeout=timeout)


async def _run_pinterest_download(
    bot,
    task_id: str,
    url: str,
    chat_id: int,
    user_id: int,
    message_id: int,
    lang: str,
    lock_token: str,
) -> None:
    """Async core of pinterest_download_task.

    Extracted to module level so the Celery task is a thin ``asyncio.run()``
    wrapper, keeping event-loop creation explicit.
    """
    with tempfile_manager(task_id) as task_dir:
        try:
            await _edit_message(
                bot, chat_id, message_id, get("download.downloading", lang)
            )

            try:
                result = await _download_pinterest_with_timeout(
                    url,
                    task_dir,
                    settings.pinterest_max_items,
                    pinterest_timeout_seconds(),
                )
            except asyncio.TimeoutError:
                logger.warning("pinterest download timed out", task_id=task_id, url=url)
                await _edit_message(
                    bot, chat_id, message_id, get("download.timeout", lang)
                )
                await _record_download_safe(
                    url,
                    "pinterest",
                    "best",
                    0,
                    DownloadStatus.FAILED,
                    user_id,
                    chat_id,
                    f"exceeded {pinterest_timeout_seconds()}s timeout",
                )
                return

            if not result.items:
                await _edit_message(
                    bot, chat_id, message_id, get("pinterest.no_media", lang)
                )
                return

            await _delete_message(bot, chat_id, message_id)

            item = result.items[0]
            file_path = Path(item.file_path)
            size_mb = item.file_size / (1024 * 1024)
            size_limit = (
                pinterest_max_file_mb()
                if item.media_type == "video"
                else send_document_limit_mb()
            )
            if size_mb > size_limit:
                await bot.send_message(
                    chat_id, get("pinterest.all_too_large", lang)
                )
                return

            title = item.title or os.path.basename(file_path)
            await send_file(
                bot,
                file_path,
                chat_id,
                lang,
                title,
                media_type=item.media_type,
            )
            await _record_download_safe(
                url,
                "pinterest",
                item.media_type,
                size_mb,
                DownloadStatus.COMPLETED,
                user_id,
                chat_id,
            )

        except PinterestNoMediaError:
            await _edit_message(
                bot, chat_id, message_id, get("pinterest.no_media", lang)
            )
            await _record_download_safe(
                url,
                "pinterest",
                "best",
                0,
                DownloadStatus.FAILED,
                user_id,
                chat_id,
                "no media found",
            )

        except PinterestDownloadError as exc:
            logger.warning("pinterest download rejected", task_id=task_id, error=str(exc))
            key = (
                "pinterest.private"
                if "private" in str(exc).lower()
                else "pinterest.invalid"
            )
            await _edit_message(bot, chat_id, message_id, get(key, lang))
            await _record_download_safe(
                url,
                "pinterest",
                "best",
                0,
                DownloadStatus.FAILED,
                user_id,
                chat_id,
                str(exc)[:500],
            )

        except Exception as exc:
            logger.exception("pinterest_download_task failed", task_id=task_id)
            try:
                await _edit_message(
                    bot, chat_id, message_id, get("errors.generic", lang)
                )
            except Exception as edit_err:
                logger.warning(
                    "failed to edit error message",
                    chat_id=chat_id, message_id=message_id,
                    error=str(edit_err),
                )
            try:
                await _record_download(
                    url,
                    "pinterest",
                    "best",
                    0,
                    DownloadStatus.FAILED,
                    user_id,
                    chat_id,
                    str(exc)[:500],
                )
            except Exception as rec_err:
                logger.exception(
                    "failed to record failed download", task_id=task_id, error=str(rec_err)
                )
        finally:
            release_user_lock_sync(user_id, lock_token, UserScenario.PINTEREST)


@app.task(bind=True, max_retries=1)
def pinterest_download_task(
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
        _run_pinterest_download(
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
        await _record_download(
            url, platform, format_id, size_mb, status, user_id, chat_id, error
        )
    except Exception:
        logger.exception(
            "failed to record download history",
            url=url,
            platform=platform,
            status=status.value,
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
                size_mb * 1024 * 1024
            )

import asyncio
import time
import uuid
from dataclasses import dataclass

import structlog
from aiogram import Bot
from aiogram.types import FSInputFile, Message

from bot.config import Settings
from bot.locale import get
from bot.metrics import (
    SOUNDCLOUD_DOWNLOADS_ENQUEUED_TOTAL,
    SOUNDCLOUD_DOWNLOAD_DURATION_SECONDS,
    SOUNDCLOUD_DOWNLOAD_FAILURES_TOTAL,
    SOUNDCLOUD_DOWNLOADS_SUCCESS_TOTAL,
    SOUNDCLOUD_DOWNLOADS_TIMEOUT_TOTAL,
    SOUNDCLOUD_METADATA_FAILURES_TOTAL,
    SOUNDCLOUD_PLAYLIST_TRACKS,
)
from bot.services.audio_cover import fetch_audio_thumbnail, send_audio_with_thumbnail_fallback
from bot.services.runtime_settings import (
    soundcloud_enabled,
    soundcloud_max_tracks,
    soundcloud_track_timeout_seconds,
)
from bot.services.soundcloud_audio import (
    SoundCloudAudioError,
    SoundCloudAudioTimeoutError,
    download_track,
    is_soundcloud_download_enabled,
)
from bot.services.soundcloud_card import build_soundcloud_card_text, build_soundcloud_keyboard
from bot.services.soundcloud_client import (
    SoundCloudError,
    SoundCloudNotFoundError,
    SoundCloudPlaylistTooLargeError,
    SoundCloudTimeoutError,
    SoundCloudUnavailableError,
    fetch_release,
)
from bot.services.soundcloud_models import NormalizedSoundCloudRelease, NormalizedSoundCloudTrack
from bot.services.soundcloud_parser import SoundCloudLink
from bot.services.tempfiles import tempfile_manager
from bot.services.user_queue import UserScenario, release_user_lock
from bot.services.download_cancel import (
    build_cancel_keyboard,
    register_download_task,
    unregister_download_task,
)

logger = structlog.get_logger()

_RELEASE_DL_PREFIX = "soundcloud:processing"


async def _reply_status(message: Message, text: str, reply_markup=None):
    if reply_markup is None:
        return await message.reply(text)
    return await message.reply(text, reply_markup=reply_markup)


async def _edit_status(status_msg, text: str, reply_markup=None, *, clear_markup: bool = False) -> None:
    if reply_markup is not None or clear_markup:
        await status_msg.edit_text(text, reply_markup=reply_markup)
    else:
        await status_msg.edit_text(text)


def _release_download_lock_key(url: str) -> str:
    return f"{_RELEASE_DL_PREFIX}:{url}"


def _release_download_lock_ttl(track_count: int, settings: Settings) -> int:
    per_track = max(soundcloud_track_timeout_seconds(), 1)
    return max(track_count, 1) * per_track + 90


async def _try_acquire_release_download_lock(
    url: str,
    track_count: int,
    settings: Settings,
) -> bool:
    from bot.services.redis_client import get_async_redis

    try:
        redis_client = await get_async_redis()
        acquired = await redis_client.set(
            _release_download_lock_key(url),
            "1",
            nx=True,
            ex=_release_download_lock_ttl(track_count, settings),
        )
        return bool(acquired)
    except Exception:
        logger.warning(
            "soundcloud release download lock unavailable, proceeding without dedup",
            url=url,
            exc_info=True,
        )
        return True


async def _release_release_download_lock(url: str) -> None:
    from bot.services.redis_client import get_async_redis

    try:
        redis_client = await get_async_redis()
        await redis_client.delete(_release_download_lock_key(url))
    except Exception:
        logger.warning("soundcloud release download lock release failed", url=url, exc_info=True)


@dataclass
class _TrackDownloadResult:
    index: int
    track: NormalizedSoundCloudTrack
    audio_path: str | None
    error: Exception | None


async def _download_one_track(
    index: int,
    track: NormalizedSoundCloudTrack,
    task_dir,
    settings: Settings,
) -> _TrackDownloadResult:
    track_dir = task_dir / f"track-{index}"
    track_dir.mkdir(parents=True, exist_ok=True)
    start = time.monotonic()
    try:
        audio_path = await asyncio.to_thread(download_track, track, track_dir, settings)
        SOUNDCLOUD_DOWNLOAD_DURATION_SECONDS.observe(time.monotonic() - start)
        return _TrackDownloadResult(index=index, track=track, audio_path=str(audio_path), error=None)
    except SoundCloudAudioTimeoutError as exc:
        SOUNDCLOUD_DOWNLOADS_TIMEOUT_TOTAL.inc()
        SOUNDCLOUD_DOWNLOAD_DURATION_SECONDS.observe(time.monotonic() - start)
        logger.exception("soundcloud track download timed out", title=track.title, index=index)
        return _TrackDownloadResult(index=index, track=track, audio_path=None, error=exc)
    except SoundCloudAudioError as exc:
        SOUNDCLOUD_DOWNLOAD_FAILURES_TOTAL.inc()
        SOUNDCLOUD_DOWNLOAD_DURATION_SECONDS.observe(time.monotonic() - start)
        logger.exception("soundcloud track download failed", title=track.title, index=index)
        return _TrackDownloadResult(index=index, track=track, audio_path=None, error=exc)


async def _send_downloaded_tracks(
    message: Message,
    release: NormalizedSoundCloudRelease,
    settings: Settings,
    lang: str,
    soundcloud_link: SoundCloudLink,
    user_id: int | None = None,
    lock_token: str = "",
) -> None:
    tracks = release.tracks
    if not tracks:
        return

    if not await _try_acquire_release_download_lock(
        soundcloud_link.url,
        len(tracks),
        settings,
    ):
        logger.info("soundcloud release already downloading", url=soundcloud_link.url)
        return

    SOUNDCLOUD_DOWNLOADS_ENQUEUED_TOTAL.inc(len(tracks))
    cancel_keyboard = build_cancel_keyboard(lang, UserScenario.SOUNDCLOUD, user_id, lock_token)
    status_msg = await _reply_status(
        message,
        get("soundcloud.download_starting", lang, total=len(tracks)),
        reply_markup=cancel_keyboard,
    )
    task_id = f"soundcloud-{message.chat.id}-{uuid.uuid4().hex[:8]}"
    bot: Bot = message.bot
    current_task = asyncio.current_task()
    if current_task is not None:
        register_download_task(UserScenario.SOUNDCLOUD, user_id, lock_token, current_task)

    try:
        with tempfile_manager(task_id) as task_dir:
            thumbnail = await fetch_audio_thumbnail(release.artwork_url, task_dir, "soundcloud-cover")
            download_tasks = [
                _download_one_track(index, track, task_dir, settings)
                for index, track in enumerate(tracks, start=1)
            ]
            results = await asyncio.gather(*download_tasks, return_exceptions=True)

            sent = 0
            for raw_result in sorted(
                (result for result in results if isinstance(result, _TrackDownloadResult)),
                key=lambda item: item.index,
            ):
                if raw_result.error is not None or raw_result.audio_path is None:
                    continue

                send_kwargs: dict = {
                    "chat_id": message.chat.id,
                    "audio": FSInputFile(raw_result.audio_path),
                    "title": raw_result.track.title,
                    "performer": raw_result.track.artist,
                }
                if thumbnail is not None:
                    send_kwargs["thumbnail"] = thumbnail
                if raw_result.track.duration_ms:
                    send_kwargs["duration"] = raw_result.track.duration_ms // 1000

                try:
                    await send_audio_with_thumbnail_fallback(bot, **send_kwargs)
                    SOUNDCLOUD_DOWNLOADS_SUCCESS_TOTAL.inc()
                except Exception:
                    logger.exception(
                        "soundcloud send_audio failed",
                        title=raw_result.track.title,
                        path=raw_result.audio_path,
                    )
                    continue

                sent += 1
                try:
                    await _edit_status(
                        status_msg,
                        get("soundcloud.download_progress", lang, current=sent, total=len(tracks)),
                        reply_markup=cancel_keyboard,
                    )
                except Exception:
                    logger.warning("soundcloud progress message edit failed", exc_info=True)

            for raw_result in results:
                if isinstance(raw_result, Exception):
                    logger.exception("unexpected soundcloud track download task failure", error=raw_result)

            try:
                if sent == 0:
                    await _edit_status(
                        status_msg,
                        get("soundcloud.download_failed", lang),
                        clear_markup=cancel_keyboard is not None,
                    )
                else:
                    await _edit_status(
                        status_msg,
                        get("soundcloud.download_done", lang, count=sent, total=len(tracks)),
                        clear_markup=cancel_keyboard is not None,
                    )
            except Exception:
                logger.warning("soundcloud final status message edit failed", exc_info=True)
    except asyncio.CancelledError:
        logger.info("soundcloud download cancelled", url=soundcloud_link.url)
        raise
    except Exception:
        logger.exception("soundcloud download flow failed", url=soundcloud_link.url)
        try:
            await _edit_status(
                status_msg,
                get("soundcloud.download_failed", lang),
                clear_markup=cancel_keyboard is not None,
            )
        except Exception:
            logger.warning("soundcloud failure status message edit failed", exc_info=True)
    finally:
        unregister_download_task(UserScenario.SOUNDCLOUD, user_id, lock_token)
        await _release_release_download_lock(soundcloud_link.url)


async def handle_soundcloud_link(
    message: Message,
    soundcloud_link: SoundCloudLink,
    settings: Settings,
    lang: str,
    lock_token: str = "",
) -> None:
    user_id = message.from_user.id if message.from_user else None

    async def _release_lock() -> None:
        if user_id and lock_token:
            await release_user_lock(user_id, lock_token, UserScenario.SOUNDCLOUD)

    if not soundcloud_enabled():
        await message.reply(get("soundcloud.disabled", lang))
        await _release_lock()
        return

    try:
        release = await fetch_release(soundcloud_link, settings)
    except SoundCloudNotFoundError:
        SOUNDCLOUD_METADATA_FAILURES_TOTAL.inc()
        await message.reply(get("soundcloud.not_found", lang))
        await _release_lock()
        return
    except SoundCloudUnavailableError:
        SOUNDCLOUD_METADATA_FAILURES_TOTAL.inc()
        await message.reply(get("soundcloud.not_found", lang))
        await _release_lock()
        return
    except SoundCloudPlaylistTooLargeError:
        await message.reply(
            get("soundcloud.playlist_too_large", lang, limit=soundcloud_max_tracks())
        )
        await _release_lock()
        return
    except SoundCloudTimeoutError:
        SOUNDCLOUD_METADATA_FAILURES_TOTAL.inc()
        await message.reply(get("soundcloud.download_timeout", lang))
        await _release_lock()
        return
    except SoundCloudError:
        SOUNDCLOUD_METADATA_FAILURES_TOTAL.inc()
        await message.reply(get("soundcloud.download_failed", lang))
        await _release_lock()
        return

    if release.tracks:
        SOUNDCLOUD_PLAYLIST_TRACKS.observe(len(release.tracks))

    text = build_soundcloud_card_text(release, lang, settings)
    keyboard = build_soundcloud_keyboard(release, lang)

    if release.artwork_url:
        await message.reply_photo(
            photo=release.artwork_url,
            caption=text,
            reply_markup=keyboard,
        )
    else:
        await message.reply(text, reply_markup=keyboard)

    await _release_lock()

    if is_soundcloud_download_enabled(settings) and release.tracks:
        asyncio.create_task(
            _send_downloaded_tracks(
                message,
                release,
                settings,
                lang,
                soundcloud_link,
                user_id=user_id,
                lock_token=lock_token,
            )
        )

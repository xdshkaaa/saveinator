import asyncio
import uuid
from dataclasses import dataclass

import structlog
from aiogram import Bot
from aiogram.types import FSInputFile, Message

from bot.config import Settings
from bot.locale import get
from bot.metrics import SOUNDCLOUD_DOWNLOADS_ENQUEUED_TOTAL
from bot.services.runtime_settings import soundcloud_track_timeout_seconds
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

logger = structlog.get_logger()

_RELEASE_DL_PREFIX = "soundcloud:processing"


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
    try:
        audio_path = await asyncio.to_thread(download_track, track, track_dir, settings)
        return _TrackDownloadResult(index=index, track=track, audio_path=str(audio_path), error=None)
    except (SoundCloudAudioTimeoutError, SoundCloudAudioError) as exc:
        logger.exception("soundcloud track download failed", title=track.title, index=index)
        return _TrackDownloadResult(index=index, track=track, audio_path=None, error=exc)


async def _send_downloaded_tracks(
    message: Message,
    release: NormalizedSoundCloudRelease,
    settings: Settings,
    lang: str,
    soundcloud_link: SoundCloudLink,
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
    status_msg = await message.reply(
        get("soundcloud.download_starting", lang, total=len(tracks))
    )
    task_id = f"soundcloud-{message.chat.id}-{uuid.uuid4().hex[:8]}"
    bot: Bot = message.bot

    try:
        with tempfile_manager(task_id) as task_dir:
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
                if raw_result.track.duration_ms:
                    send_kwargs["duration"] = raw_result.track.duration_ms // 1000

                try:
                    await bot.send_audio(**send_kwargs)
                except Exception:
                    logger.exception(
                        "soundcloud send_audio failed",
                        title=raw_result.track.title,
                        path=raw_result.audio_path,
                    )
                    continue

                sent += 1
                try:
                    await status_msg.edit_text(
                        get("soundcloud.download_progress", lang, current=sent, total=len(tracks))
                    )
                except Exception:
                    logger.warning("soundcloud progress message edit failed", exc_info=True)

            for raw_result in results:
                if isinstance(raw_result, Exception):
                    logger.exception("unexpected soundcloud track download task failure", error=raw_result)

            try:
                if sent == 0:
                    await status_msg.edit_text(get("soundcloud.download_failed", lang))
                else:
                    await status_msg.edit_text(
                        get("soundcloud.download_done", lang, count=sent, total=len(tracks))
                    )
            except Exception:
                logger.warning("soundcloud final status message edit failed", exc_info=True)
    except Exception:
        logger.exception("soundcloud download flow failed", url=soundcloud_link.url)
        try:
            await status_msg.edit_text(get("soundcloud.download_failed", lang))
        except Exception:
            logger.warning("soundcloud failure status message edit failed", exc_info=True)
    finally:
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

    if not settings.soundcloud_enabled:
        await message.reply(get("soundcloud.disabled", lang))
        await _release_lock()
        return

    try:
        release = await fetch_release(soundcloud_link, settings)
    except SoundCloudNotFoundError:
        await message.reply(get("soundcloud.not_found", lang))
        await _release_lock()
        return
    except SoundCloudUnavailableError:
        await message.reply(get("soundcloud.not_found", lang))
        await _release_lock()
        return
    except SoundCloudPlaylistTooLargeError:
        await message.reply(
            get("soundcloud.playlist_too_large", lang, limit=settings.soundcloud_max_tracks)
        )
        await _release_lock()
        return
    except SoundCloudTimeoutError:
        await message.reply(get("soundcloud.download_timeout", lang))
        await _release_lock()
        return
    except SoundCloudError:
        await message.reply(get("soundcloud.download_failed", lang))
        await _release_lock()
        return

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
            _send_downloaded_tracks(message, release, settings, lang, soundcloud_link)
        )

import asyncio
import uuid
from dataclasses import dataclass

import structlog
from aiogram import Bot
from aiogram.types import FSInputFile, Message

from bot.config import Settings
from bot.locale import get
from bot.services.spotify_cache import (
    get_cached_youtube_video_id,
    set_cached_youtube_video_id,
)
from bot.services.audio_cover import fetch_audio_thumbnail, send_audio_with_thumbnail_fallback
from bot.services.spotify_card import build_spotify_card_text, build_spotify_keyboard
from bot.services.spotify_client import (
    SpotifyApiError,
    SpotifyAuthError,
    SpotifyNotFoundError,
    SpotifyRateLimitError,
    SpotifyTimeoutError,
    fetch_release,
)
from bot.services.spotify_models import NormalizedSpotifyRelease, NormalizedSpotifyTrack
from bot.services.spotify_parser import SpotifyLink
from bot.services.runtime_settings import spotify_track_timeout_seconds
from bot.services.tempfiles import tempfile_manager
from bot.services.user_queue import UserScenario, release_user_lock
from bot.services.download_cancel import (
    build_cancel_keyboard,
    register_download_task,
    unregister_download_task,
)
from bot.services.music_download_progress import edit_download_status, run_ordered_release_download
from bot.services.youtube_audio import (
    YoutubeAudioError,
    YoutubeAudioNotFoundError,
    YoutubeAudioTimeoutError,
    build_track_search_query,
    download_track_from_youtube,
    is_spotify_download_enabled,
    resolve_youtube_video_id,
    youtube_watch_url,
)

logger = structlog.get_logger()

_RELEASE_DL_PREFIX = "spotify:processing"


async def _reply_status(message: Message, text: str, reply_markup=None):
    if reply_markup is None:
        return await message.reply(text)
    return await message.reply(text, reply_markup=reply_markup)


def _release_download_lock_key(link_type: str, resource_id: str) -> str:
    return f"{_RELEASE_DL_PREFIX}:{link_type}:{resource_id}"


def _release_download_lock_ttl(track_count: int, settings: Settings) -> int:
    per_track = max(spotify_track_timeout_seconds(), 1)
    concurrency = max(settings.spotify_download_concurrency, 1)
    batches = (max(track_count, 1) + concurrency - 1) // concurrency
    return batches * per_track + 90


async def _try_acquire_release_download_lock(
    link_type: str,
    resource_id: str,
    track_count: int,
    settings: Settings,
) -> bool:
    from bot.services.redis_client import get_async_redis

    try:
        redis_client = await get_async_redis()
        acquired = await redis_client.set(
            _release_download_lock_key(link_type, resource_id),
            "1",
            nx=True,
            ex=_release_download_lock_ttl(track_count, settings),
        )
        return bool(acquired)
    except Exception:
        logger.warning(
            "release download lock unavailable, proceeding without dedup",
            link_type=link_type,
            resource_id=resource_id,
            exc_info=True,
        )
        return True


async def _release_release_download_lock(link_type: str, resource_id: str) -> None:
    from bot.services.redis_client import get_async_redis

    try:
        redis_client = await get_async_redis()
        await redis_client.delete(_release_download_lock_key(link_type, resource_id))
    except Exception:
        logger.warning(
            "release download lock release failed",
            link_type=link_type,
            resource_id=resource_id,
            exc_info=True,
        )


@dataclass
class _TrackDownloadResult:
    index: int
    track: NormalizedSpotifyTrack
    audio_path: str | None
    error: Exception | None


async def _download_track_audio(
    index: int,
    track: NormalizedSpotifyTrack,
    track_dir,
    settings: Settings,
    semaphore: asyncio.Semaphore,
    on_download_start,
) -> str | None:
    query = build_track_search_query(track)
    video_id = await get_cached_youtube_video_id(query)

    if not video_id:
        video_id = await asyncio.to_thread(resolve_youtube_video_id, query, settings)
        if video_id:
            await set_cached_youtube_video_id(
                query,
                video_id,
                settings.youtube_search_cache_ttl_seconds,
            )

    if not video_id:
        raise YoutubeAudioNotFoundError(f"No YouTube match for: {query}")

    youtube_url = youtube_watch_url(video_id)

    async with semaphore:
        await on_download_start(index, track)
        audio_path = await asyncio.to_thread(
            download_track_from_youtube,
            track,
            track_dir,
            settings,
            youtube_url=youtube_url,
        )

    return str(audio_path)


async def _download_one_track(
    index: int,
    track: NormalizedSpotifyTrack,
    task_dir,
    settings: Settings,
    semaphore: asyncio.Semaphore,
    on_download_start,
) -> _TrackDownloadResult:
    track_dir = task_dir / f"track-{index}"
    track_dir.mkdir(parents=True, exist_ok=True)
    try:
        audio_path = await _download_track_audio(
            index,
            track,
            track_dir,
            settings,
            semaphore,
            on_download_start,
        )
        return _TrackDownloadResult(index=index, track=track, audio_path=audio_path, error=None)
    except (YoutubeAudioTimeoutError, YoutubeAudioError, YoutubeAudioNotFoundError) as exc:
        logger.exception("track download failed", title=track.title, index=index)
        return _TrackDownloadResult(index=index, track=track, audio_path=None, error=exc)


async def _send_downloaded_tracks(
    message: Message,
    release: NormalizedSpotifyRelease,
    settings: Settings,
    lang: str,
    spotify_link: SpotifyLink,
    user_id: int | None = None,
    lock_token: str = "",
) -> None:
    tracks = release.tracks
    if not tracks:
        return

    if not await _try_acquire_release_download_lock(
        spotify_link.type,
        spotify_link.id,
        len(tracks),
        settings,
    ):
        logger.info(
            "spotify release already downloading",
            link_type=spotify_link.type,
            resource_id=spotify_link.id,
        )
        return

    cancel_keyboard = build_cancel_keyboard(lang, UserScenario.SPOTIFY, user_id, lock_token)
    status_msg = await _reply_status(
        message,
        get("spotify.download_starting", lang, total=len(tracks)),
        reply_markup=cancel_keyboard,
    )
    task_id = f"spotify-{message.chat.id}-{uuid.uuid4().hex[:8]}"
    bot: Bot = message.bot
    semaphore = asyncio.Semaphore(max(settings.spotify_download_concurrency, 1))
    current_task = asyncio.current_task()
    if current_task is not None:
        register_download_task(UserScenario.SPOTIFY, user_id, lock_token, current_task)

    try:
        with tempfile_manager(task_id) as task_dir:
            thumbnail = await fetch_audio_thumbnail(release.cover_url, task_dir, "spotify-cover")

            async def send_track(result: _TrackDownloadResult) -> bool:
                send_kwargs: dict = {
                    "chat_id": message.chat.id,
                    "audio": FSInputFile(result.audio_path),
                    "title": result.track.title,
                    "performer": result.track.artists,
                }
                if thumbnail is not None:
                    send_kwargs["thumbnail"] = thumbnail
                await send_audio_with_thumbnail_fallback(bot, **send_kwargs)
                return True

            sent, _results = await run_ordered_release_download(
                tracks=tracks,
                status_msg=status_msg,
                lang=lang,
                locale_prefix="spotify",
                cancel_keyboard=cancel_keyboard,
                download_fn=lambda index, track, on_download_start: _download_one_track(
                    index, track, task_dir, settings, semaphore, on_download_start
                ),
                send_fn=send_track,
            )

            if sent == 0:
                await edit_download_status(
                    status_msg,
                    get("spotify.download_none_found", lang),
                    clear_markup=cancel_keyboard is not None,
                )
            else:
                await edit_download_status(
                    status_msg,
                    get("spotify.download_done", lang, count=sent, total=len(tracks)),
                    clear_markup=cancel_keyboard is not None,
                )
    except YoutubeAudioNotFoundError:
        await edit_download_status(
            status_msg,
            get("spotify.download_not_available", lang),
            clear_markup=cancel_keyboard is not None,
        )
    except asyncio.CancelledError:
        logger.info(
            "spotify download cancelled",
            link_type=spotify_link.type,
            resource_id=spotify_link.id,
        )
        raise
    finally:
        unregister_download_task(UserScenario.SPOTIFY, user_id, lock_token)
        await _release_release_download_lock(spotify_link.type, spotify_link.id)


async def reply_spotify_link(
    message: Message,
    spotify_link: SpotifyLink,
    settings: Settings,
    lang: str,
    lock_token: str = "",
) -> None:
    user_id = message.from_user.id if message.from_user else None

    async def _release_lock() -> None:
        if user_id and lock_token:
            await release_user_lock(user_id, lock_token, UserScenario.SPOTIFY)

    if not settings.spotify_enabled:
        await message.reply(get("spotify.disabled", lang))
        await _release_lock()
        return

    if not settings.spotify_client_id or not settings.spotify_client_secret:
        await message.reply(get("spotify.not_configured", lang))
        await _release_lock()
        return

    try:
        release = await fetch_release(
            spotify_link.type,
            spotify_link.id,
            settings,
        )
    except SpotifyNotFoundError:
        await message.reply(get("spotify.not_found", lang))
        await _release_lock()
        return
    except SpotifyAuthError:
        await message.reply(get("spotify.not_configured", lang))
        await _release_lock()
        return
    except (SpotifyRateLimitError, SpotifyTimeoutError, SpotifyApiError):
        await message.reply(get("spotify.api_error", lang))
        await _release_lock()
        return

    text = build_spotify_card_text(release, lang, settings)
    keyboard = build_spotify_keyboard(release, lang)

    if release.cover_url:
        await message.reply_photo(
            photo=release.cover_url,
            caption=text,
            reply_markup=keyboard,
        )
    else:
        await message.reply(text, reply_markup=keyboard)

    await _release_lock()

    if is_spotify_download_enabled(settings) and release.tracks:
        asyncio.create_task(
            _send_downloaded_tracks(
                message,
                release,
                settings,
                lang,
                spotify_link,
                user_id=user_id,
                lock_token=lock_token,
            )
        )

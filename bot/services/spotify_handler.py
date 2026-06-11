import asyncio
import uuid

import structlog
from aiogram import Bot
from aiogram.types import FSInputFile, Message

from bot.config import Settings
from bot.locale import get
from bot.services.spotify_card import build_spotify_card_text, build_spotify_keyboard
from bot.services.spotify_client import (
    SpotifyApiError,
    SpotifyAuthError,
    SpotifyNotFoundError,
    SpotifyRateLimitError,
    SpotifyTimeoutError,
    fetch_release,
)
from bot.services.spotify_models import NormalizedSpotifyRelease
from bot.services.spotify_parser import SpotifyLink
from bot.services.tempfiles import tempfile_manager
from bot.services.user_queue import UserScenario, extend_user_lock, release_user_lock
from bot.services.youtube_audio import (
    YoutubeAudioError,
    YoutubeAudioNotFoundError,
    YoutubeAudioTimeoutError,
    download_track_from_youtube,
    is_spotify_download_enabled,
)

logger = structlog.get_logger()


async def _send_downloaded_tracks(
    message: Message,
    release: NormalizedSpotifyRelease,
    settings: Settings,
    lang: str,
    lock_token: str,
) -> None:
    tracks = release.tracks
    if not tracks:
        if lock_token and message.from_user:
            await release_user_lock(message.from_user.id, lock_token, UserScenario.SPOTIFY)
        return

    user_id = message.from_user.id if message.from_user else None
    if user_id and lock_token:
        await extend_user_lock(
            user_id,
            lock_token,
            UserScenario.SPOTIFY,
            track_count=len(tracks),
        )

    status_msg = await message.reply(
        get("spotify.download_starting", lang, total=len(tracks))
    )
    task_id = f"spotify-{message.chat.id}-{uuid.uuid4().hex[:8]}"
    bot: Bot = message.bot

    try:
        with tempfile_manager(task_id) as task_dir:
            sent = 0
            for index, track in enumerate(tracks, start=1):
                track_dir = task_dir / f"track-{index}"
                try:
                    audio_path = await asyncio.to_thread(
                        download_track_from_youtube,
                        track,
                        track_dir,
                        settings,
                    )
                    await bot.send_audio(
                        chat_id=message.chat.id,
                        audio=FSInputFile(audio_path),
                        title=track.title,
                        performer=track.artists,
                    )
                    sent += 1
                    await status_msg.edit_text(
                        get("spotify.download_progress", lang, current=sent, total=len(tracks))
                    )
                except (YoutubeAudioTimeoutError, YoutubeAudioError):
                    logger.exception("track download failed", title=track.title)

            if sent == 0:
                await status_msg.edit_text(get("spotify.download_none_found", lang))
            else:
                await status_msg.edit_text(
                    get("spotify.download_done", lang, count=sent, total=len(tracks))
                )
    except YoutubeAudioNotFoundError:
        await status_msg.edit_text(get("spotify.download_not_available", lang))
    finally:
        if user_id and lock_token:
            await release_user_lock(user_id, lock_token, UserScenario.SPOTIFY)


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
        release = await asyncio.to_thread(
            fetch_release,
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

    if is_spotify_download_enabled(settings) and release.tracks:
        asyncio.create_task(
            _send_downloaded_tracks(message, release, settings, lang, lock_token)
        )
        return

    await _release_lock()

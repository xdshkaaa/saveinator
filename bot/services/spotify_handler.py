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
from bot.services.spotify_dl import (
    SpotifyDlError,
    SpotifyDlNotFoundError,
    SpotifyDlTimeoutError,
    is_spotify_download_enabled,
    run_spotify_dl,
)
from bot.services.spotify_models import NormalizedSpotifyRelease
from bot.services.spotify_parser import SpotifyLink
from bot.services.tempfiles import tempfile_manager

logger = structlog.get_logger()


def _spotify_url_for_link(spotify_link: SpotifyLink) -> str:
    return f"https://open.spotify.com/{spotify_link.type}/{spotify_link.id}"


async def _send_downloaded_tracks(
    message: Message,
    release: NormalizedSpotifyRelease,
    spotify_url: str,
    settings: Settings,
    lang: str,
) -> None:
    total = max(len(release.tracks), 1)
    status_msg = await message.reply(
        get("spotify.download_starting", lang, total=total)
    )
    task_id = f"spotify-{message.chat.id}-{uuid.uuid4().hex[:8]}"
    bot: Bot = message.bot

    try:
        with tempfile_manager(task_id) as task_dir:
            tracks = await asyncio.to_thread(
                run_spotify_dl,
                spotify_url,
                task_dir,
                settings,
            )

            sent = 0
            for track in tracks:
                try:
                    await bot.send_audio(
                        chat_id=message.chat.id,
                        audio=FSInputFile(track.path),
                        title=track.title,
                    )
                    sent += 1
                    await status_msg.edit_text(
                        get("spotify.download_progress", lang, current=sent, total=len(tracks))
                    )
                except Exception:
                    logger.exception("failed to send spotify-dl track", title=track.title)

            if sent == 0:
                await status_msg.edit_text(get("spotify.download_none_found", lang))
            else:
                await status_msg.edit_text(
                    get("spotify.download_done", lang, count=sent, total=len(tracks))
                )
    except SpotifyDlNotFoundError:
        await status_msg.edit_text(get("spotify.download_not_available", lang))
    except SpotifyDlTimeoutError:
        await status_msg.edit_text(get("spotify.download_timeout", lang))
    except SpotifyDlError:
        logger.exception("spotify-dl download failed", url=spotify_url)
        await status_msg.edit_text(get("spotify.download_failed", lang))


async def reply_spotify_link(
    message: Message,
    spotify_link: SpotifyLink,
    settings: Settings,
    lang: str,
) -> None:
    if not settings.spotify_enabled:
        await message.reply(get("spotify.disabled", lang))
        return

    if not settings.spotify_client_id or not settings.spotify_client_secret:
        await message.reply(get("spotify.not_configured", lang))
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
        return
    except SpotifyAuthError:
        await message.reply(get("spotify.not_configured", lang))
        return
    except (SpotifyRateLimitError, SpotifyTimeoutError, SpotifyApiError):
        await message.reply(get("spotify.api_error", lang))
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

    if is_spotify_download_enabled(settings):
        spotify_url = _spotify_url_for_link(spotify_link)
        asyncio.create_task(
            _send_downloaded_tracks(message, release, spotify_url, settings, lang)
        )

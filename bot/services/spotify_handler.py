import asyncio
from pathlib import Path

import structlog
from aiogram import Bot
from aiogram.types import Message

from bot.config import Settings
from bot.locale import get
from bot.services.audio_providers import (
    get_download_provider,
    get_search_provider,
    has_audio_download_pipeline,
)
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

logger = structlog.get_logger()


def build_track_search_query(track: NormalizedSpotifyTrack) -> str:
    return f"{track.artists} - {track.title}"


async def _download_tracks_via_providers(
    message: Message,
    release: NormalizedSpotifyRelease,
    lang: str,
) -> None:
    search_provider = get_search_provider()
    download_provider = get_download_provider()
    if search_provider is None or download_provider is None:
        return

    total = len(release.tracks)
    if total == 0:
        return

    status_msg = await message.reply(get("spotify.download_progress", lang, current=0, total=total))
    output_dir = Path("/tmp") / f"spotify-{message.chat.id}-{status_msg.message_id}"
    output_dir.mkdir(parents=True, exist_ok=True)

    completed = 0
    for track in release.tracks:
        query = build_track_search_query(track)
        try:
            result = await search_provider.search_track(track.artists, track.title)
            if result is None:
                logger.info("spotify track search miss", query=query)
                continue

            downloaded = await download_provider.download(result, str(output_dir))
            bot: Bot = message.bot
            with downloaded.path.open("rb") as audio_file:
                await bot.send_audio(
                    chat_id=message.chat.id,
                    audio=audio_file,
                    title=downloaded.title,
                )
            completed += 1
            await status_msg.edit_text(
                get("spotify.download_progress", lang, current=completed, total=total)
            )
        except Exception:
            logger.exception("spotify optional download failed", query=query)

    if completed == 0:
        await status_msg.edit_text(get("spotify.download_none_found", lang))
    else:
        await status_msg.edit_text(get("spotify.download_done", lang, count=completed, total=total))


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

    text = build_spotify_card_text(release, lang)
    keyboard = build_spotify_keyboard(release, lang)

    if release.cover_url:
        await message.reply_photo(
            photo=release.cover_url,
            caption=text,
            reply_markup=keyboard,
        )
    else:
        await message.reply(text, reply_markup=keyboard)

    if has_audio_download_pipeline():
        await _download_tracks_via_providers(message, release, lang)

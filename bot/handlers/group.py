import asyncio

from aiogram import Router, F
from aiogram.types import Message
from aiogram.enums import ChatType

from bot.config import settings
from bot.locale import get
from bot.services.link_parser import extract_urls
from bot.services.spotify_card import build_spotify_card_text, build_spotify_keyboard
from bot.services.spotify_client import (
    SpotifyApiError,
    SpotifyAuthError,
    SpotifyNotFoundError,
    SpotifyRateLimitError,
    SpotifyTimeoutError,
    fetch_album,
)
from db.models import Platform
from workers.tasks import download_and_send_task

group_router = Router()
group_router.message.filter(F.chat.type.in_([ChatType.PRIVATE, ChatType.GROUP, ChatType.SUPERGROUP]))


async def _reply_spotify_album(message: Message, album_id: str, lang: str) -> None:
    if not settings.spotify_enabled:
        await message.reply(get("spotify.disabled", lang))
        return

    if not settings.spotify_client_id or not settings.spotify_client_secret:
        await message.reply(get("spotify.not_configured", lang))
        return

    try:
        album = await asyncio.to_thread(fetch_album, album_id, settings)
    except SpotifyNotFoundError:
        await message.reply(get("spotify.not_found", lang))
        return
    except SpotifyAuthError:
        await message.reply(get("spotify.not_configured", lang))
        return
    except (SpotifyRateLimitError, SpotifyTimeoutError, SpotifyApiError):
        await message.reply(get("spotify.api_error", lang))
        return

    text = build_spotify_card_text(album, lang)
    keyboard = build_spotify_keyboard(album, lang)

    if album.cover_url:
        await message.reply_photo(
            photo=album.cover_url,
            caption=text,
            reply_markup=keyboard,
        )
        return

    await message.reply(text, reply_markup=keyboard)


@group_router.message(F.text)
async def handle_group_message(message: Message, lang: str = "en"):
    if not message.text:
        return

    urls = extract_urls(message.text)
    if not urls:
        return

    parsed_link = urls[0]
    platform = parsed_link.platform
    url = parsed_link.url

    if platform == Platform.SPOTIFY:
        if not parsed_link.spotify_album_id:
            await message.reply(get("errors.unsupported", lang))
            return
        await _reply_spotify_album(message, parsed_link.spotify_album_id, lang)
        return

    if platform.value == "unknown":
        await message.reply(get("errors.unsupported", lang))
        return

    status_msg = await message.reply(get("download.downloading", lang))

    download_and_send_task.delay(
        url=url,
        platform=platform.value,
        chat_id=message.chat.id,
        user_id=message.from_user.id if message.from_user else 0,
        message_id=status_msg.message_id,
        lang=lang,
    )

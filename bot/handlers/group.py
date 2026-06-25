from aiogram import Router, F
from aiogram.types import Message
from aiogram.enums import ChatType

from bot.config import settings
from bot.locale import get
from bot.metrics import (
    DOWNLOADS_ENQUEUED_TOTAL,
    SOUNDCLOUD_REQUESTS_TOTAL,
    SPOTIFY_REQUESTS_TOTAL,
    USER_QUEUE_REJECTED_TOTAL,
)
from bot.services.link_parser import extract_urls, is_youtube_shorts
from bot.services.runtime_settings import soundcloud_max_tracks
from bot.services.soundcloud_handler import handle_soundcloud_link
from bot.services.spotify_handler import reply_spotify_link
from bot.services.download_cancel import build_download_queue_button
from bot.services.user_queue import UserScenario, acquire_user_lock
from bot.services.user_settings import get_or_create_user_settings
from bot.services.youtube_keyboards import format_ratio_label, get_quality_keyboard, get_ratio_keyboard
from bot.services.youtube_session import (
    YoutubePendingSession,
    save_youtube_session,
)
from bot.handlers.youtube import start_youtube_quality_menu
from db.models import Platform
from workers.tasks import download_and_send_task
from workers.pinterest_task import pinterest_download_task
from workers.tiktok_task import tiktok_download_task

group_router = Router()
group_router.message.filter(F.chat.type.in_([ChatType.PRIVATE, ChatType.GROUP, ChatType.SUPERGROUP]))


async def _acquire_scenario_lock(
    message: Message,
    lang: str,
    scenario: UserScenario,
    *,
    track_count: int = 0,
) -> str | None:
    user_id = message.from_user.id if message.from_user else None
    if not user_id:
        return ""

    token = await acquire_user_lock(
        user_id,
        scenario,
        track_count=track_count,
    )
    if token is None:
        await message.reply(
            get("errors.busy", lang),
            reply_markup=build_download_queue_button(lang, user_id),
        )
        USER_QUEUE_REJECTED_TOTAL.labels(scenario=scenario.value).inc()
        return None
    return token


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
    user_id = message.from_user.id if message.from_user else 0

    if platform == Platform.SPOTIFY:
        if not parsed_link.spotify_link:
            await message.reply(get("errors.unsupported", lang))
            return
        lock_token = await _acquire_scenario_lock(
            message,
            lang,
            UserScenario.SPOTIFY,
            track_count=settings.spotify_lock_max_tracks,
        )
        if lock_token is None:
            return
        SPOTIFY_REQUESTS_TOTAL.inc()
        DOWNLOADS_ENQUEUED_TOTAL.labels(platform="spotify").inc()
        await reply_spotify_link(
            message,
            parsed_link.spotify_link,
            settings,
            lang,
            lock_token=lock_token,
        )
        return

    if platform == Platform.SOUNDCLOUD:
        if not parsed_link.soundcloud_link:
            await message.reply(get("errors.unsupported", lang))
            return
        lock_token = await _acquire_scenario_lock(
            message,
            lang,
            UserScenario.SOUNDCLOUD,
            track_count=soundcloud_max_tracks(),
        )
        if lock_token is None:
            return
        SOUNDCLOUD_REQUESTS_TOTAL.inc()
        DOWNLOADS_ENQUEUED_TOTAL.labels(platform="soundcloud").inc()
        await handle_soundcloud_link(
            message,
            parsed_link.soundcloud_link,
            settings,
            lang,
            lock_token=lock_token,
        )
        return

    if platform == Platform.PINTEREST:
        if not settings.pinterest_enabled:
            await message.reply(get("pinterest.disabled", lang))
            return
        lock_token = await _acquire_scenario_lock(message, lang, UserScenario.PINTEREST)
        if lock_token is None:
            return
        status_msg = await message.reply(get("download.downloading", lang))
        DOWNLOADS_ENQUEUED_TOTAL.labels(platform="pinterest").inc()
        pinterest_download_task.delay(
            url=url,
            chat_id=message.chat.id,
            user_id=user_id,
            message_id=status_msg.message_id,
            lang=lang,
            lock_token=lock_token,
        )
        return

    if platform == Platform.TIKTOK:
        lock_token = await _acquire_scenario_lock(message, lang, UserScenario.TIKTOK)
        if lock_token is None:
            return
        status_msg = await message.reply(get("download.downloading", lang))
        DOWNLOADS_ENQUEUED_TOTAL.labels(platform="tiktok").inc()
        tiktok_download_task.delay(
            url=url,
            chat_id=message.chat.id,
            user_id=user_id,
            message_id=status_msg.message_id,
            lang=lang,
            lock_token=lock_token,
        )
        return

    if platform.value == "unknown":
        from bot.services.tiktok_parser import is_tiktok_non_content_url

        if is_tiktok_non_content_url(url):
            await message.reply(get("tiktok.invalid_link", lang))
            return
        await message.reply(get("errors.unsupported", lang))
        return

    if platform == Platform.YOUTUBE:
        user_settings = await get_or_create_user_settings(user_id)
        auto_quality = None
        auto_ratio = None
        if user_settings.youtube_quality != "ask":
            auto_quality = int(user_settings.youtube_quality)
        if user_settings.youtube_ratio != "ask":
            auto_ratio = user_settings.youtube_ratio
        if is_youtube_shorts(url):
            auto_ratio = "9_16"

        if auto_quality is not None and auto_ratio is not None:
            status_msg = await message.reply(
                get(
                    "youtube.processing",
                    lang,
                    quality=auto_quality,
                    ratio=format_ratio_label(auto_ratio),
                ),
            )
            DOWNLOADS_ENQUEUED_TOTAL.labels(platform="youtube").inc()
            send_msg = await message.bot.send_message(
                chat_id=message.chat.id,
                text=get("download.downloading", lang),
            )
            download_and_send_task.delay(
                url=url,
                platform="youtube",
                chat_id=message.chat.id,
                user_id=user_id,
                message_id=send_msg.message_id,
                lang=lang,
                quality=auto_quality,
                aspect_ratio=auto_ratio,
            )
            return

        if auto_quality is not None:
            session = YoutubePendingSession(
                user_id=user_id,
                url=url,
                chat_id=message.chat.id,
                message_id=0,
                lang=lang,
                quality=auto_quality,
            )
            await save_youtube_session(session)
            status_msg = await message.reply(
                get("youtube.choose_ratio", lang),
                reply_markup=get_ratio_keyboard(),
            )
            return

        status_msg = await message.reply(
            get("youtube.choose_quality", lang),
            reply_markup=get_quality_keyboard(),
        )
        await start_youtube_quality_menu(
            user_id=user_id,
            url=url,
            chat_id=message.chat.id,
            message_id=status_msg.message_id,
            lang=lang,
        )
        return

    lock_token = await _acquire_scenario_lock(message, lang, UserScenario.VIDEO)
    if lock_token is None:
        return

    status_msg = await message.reply(get("download.downloading", lang))

    DOWNLOADS_ENQUEUED_TOTAL.labels(platform=platform.value).inc()
    download_and_send_task.delay(
        url=url,
        platform=platform.value,
        chat_id=message.chat.id,
        user_id=user_id,
        message_id=status_msg.message_id,
        lang=lang,
        lock_token=lock_token,
        x_status_id=parsed_link.x_status_id,
    )

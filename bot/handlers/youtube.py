from aiogram import Bot, F, Router
from aiogram.types import CallbackQuery

from bot.locale import get
from bot.metrics import DOWNLOADS_ENQUEUED_TOTAL
from bot.services.youtube_keyboards import format_ratio_label, get_ratio_keyboard
from bot.services.youtube_session import (
    YoutubePendingSession,
    clear_youtube_session,
    get_youtube_session,
    save_youtube_session,
    update_youtube_quality,
)
from workers.tasks import download_and_send_task

youtube_router = Router()

_VALID_QUALITIES = {1080, 720, 480}
_VALID_RATIOS = {"16_9", "21_9", "9_16"}


async def start_youtube_quality_menu(
    *,
    user_id: int,
    url: str,
    chat_id: int,
    message_id: int,
    lang: str,
) -> None:
    session = YoutubePendingSession(
        user_id=user_id,
        url=url,
        chat_id=chat_id,
        message_id=message_id,
        lang=lang,
    )
    await save_youtube_session(session)


@youtube_router.callback_query(F.data.startswith("quality:"))
async def handle_quality_choice(callback: CallbackQuery, lang: str = "en"):
    user_id = callback.from_user.id if callback.from_user else None
    if not user_id or not callback.message:
        await callback.answer()
        return

    session = await get_youtube_session(user_id)
    if session is None or session.user_id != user_id:
        await callback.answer(get("youtube.session_expired", lang), show_alert=True)
        return

    try:
        quality = int(callback.data.split(":", 1)[1])
    except (IndexError, ValueError):
        await callback.answer()
        return

    if quality not in _VALID_QUALITIES:
        await callback.answer()
        return

    session = await update_youtube_quality(user_id, quality)
    if session is None:
        await callback.answer(get("youtube.session_expired", lang), show_alert=True)
        return

    await callback.message.edit_text(
        get("youtube.choose_ratio", session.lang),
        reply_markup=get_ratio_keyboard(),
    )
    await callback.answer()


@youtube_router.callback_query(F.data.startswith("ratio:"))
async def handle_ratio_choice(callback: CallbackQuery, lang: str = "en"):
    user_id = callback.from_user.id if callback.from_user else None
    if not user_id or not callback.message:
        await callback.answer()
        return

    session = await get_youtube_session(user_id)
    if session is None or session.user_id != user_id:
        await callback.answer(get("youtube.session_expired", lang), show_alert=True)
        return

    if session.quality is None:
        await callback.answer(get("youtube.session_expired", lang), show_alert=True)
        return

    try:
        aspect_ratio = callback.data.split(":", 1)[1]
    except IndexError:
        await callback.answer()
        return

    if aspect_ratio not in _VALID_RATIOS:
        await callback.answer()
        return

    ratio_label = format_ratio_label(aspect_ratio)
    await callback.message.edit_text(
        get(
            "youtube.processing",
            session.lang,
            quality=session.quality,
            ratio=ratio_label,
        ),
        reply_markup=None,
    )
    await callback.answer()

    bot: Bot = callback.bot
    await start_youtube_download(session, aspect_ratio, user_id, bot)


async def start_youtube_download(
    session: YoutubePendingSession,
    aspect_ratio: str,
    user_id: int,
    bot: Bot,
) -> None:
    await clear_youtube_session(user_id)

    status_msg = await bot.send_message(
        chat_id=session.chat_id,
        text=get("download.downloading", session.lang),
    )

    DOWNLOADS_ENQUEUED_TOTAL.labels(platform="youtube").inc()
    download_and_send_task.delay(
        url=session.url,
        platform="youtube",
        chat_id=session.chat_id,
        user_id=user_id,
        message_id=status_msg.message_id,
        lang=session.lang,
        quality=session.quality,
        aspect_ratio=aspect_ratio,
    )

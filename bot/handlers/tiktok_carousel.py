from aiogram import F, Router
from aiogram.types import CallbackQuery

from bot.locale import get
from bot.metrics import DOWNLOADS_ENQUEUED_TOTAL
from bot.services.tiktok_carousel_session import get_tiktok_carousel_session
from bot.services.tiktok_keyboards import parse_carousel_images_callback
from bot.services.user_queue import UserScenario, acquire_user_lock
from workers.tiktok_task import tiktok_carousel_images_task

tiktok_carousel_router = Router()


@tiktok_carousel_router.callback_query(F.data.startswith("ttk:img:"))
async def download_carousel_images(callback: CallbackQuery, lang: str = "en") -> None:
    if not callback.from_user or not callback.message:
        await callback.answer()
        return

    parsed = parse_carousel_images_callback(callback.data)
    if parsed is None:
        await callback.answer()
        return

    if parsed.user_id != callback.from_user.id:
        await callback.answer(get("download.cancel_not_allowed", lang), show_alert=True)
        return

    session = await get_tiktok_carousel_session(parsed.token)
    if session is None or session.user_id != callback.from_user.id:
        await callback.answer(
            get("tiktok.carousel_images_unavailable", lang),
            show_alert=True,
        )
        return

    lock_token = await acquire_user_lock(callback.from_user.id, UserScenario.TIKTOK)
    if lock_token is None:
        await callback.answer(get("errors.busy", lang), show_alert=True)
        return

    await callback.answer(get("tiktok.carousel_images_downloading", lang))

    try:
        await callback.message.edit_reply_markup(reply_markup=None)
    except Exception:
        pass

    DOWNLOADS_ENQUEUED_TOTAL.labels(platform="tiktok").inc()
    tiktok_carousel_images_task.delay(
        url=session.url,
        chat_id=session.chat_id,
        user_id=session.user_id,
        lang=session.lang,
        lock_token=lock_token,
        session_token=session.token,
    )

from aiogram import F, Router
from aiogram.types import CallbackQuery

from bot.locale import get
from bot.services.download_cancel import (
    build_active_downloads_keyboard,
    cancel_download_task,
    parse_cancel_callback_data,
    parse_queue_callback_data,
)
from bot.services.user_queue import get_active_user_download, release_user_lock

download_cancel_router = Router()


@download_cancel_router.callback_query(F.data.startswith("dlc:"))
async def cancel_download(callback: CallbackQuery, lang: str = "en") -> None:
    data = parse_cancel_callback_data(callback.data)
    if data is None:
        await callback.answer(get("download.cancel_unavailable", lang), show_alert=True)
        return

    requester_id = callback.from_user.id if callback.from_user else None
    if requester_id != data.user_id:
        await callback.answer(get("download.cancel_not_allowed", lang), show_alert=True)
        return

    cancel_download_task(data.scenario, data.user_id, data.token)
    await release_user_lock(data.user_id, data.token, data.scenario)

    if callback.message:
        await callback.message.edit_text(get("download.cancelled", lang), reply_markup=None)
    await callback.answer(get("download.cancel_done", lang))


@download_cancel_router.callback_query(F.data.startswith("dlq:"))
async def show_download_queue(callback: CallbackQuery, lang: str = "en") -> None:
    user_id = parse_queue_callback_data(callback.data)
    if user_id is None:
        await callback.answer(get("download.queue_unavailable", lang), show_alert=True)
        return

    requester_id = callback.from_user.id if callback.from_user else None
    if requester_id != user_id:
        await callback.answer(get("download.queue_not_allowed", lang), show_alert=True)
        return

    active = await get_active_user_download(user_id)
    if active is None:
        if callback.message:
            await callback.message.edit_text(get("download.queue_empty", lang), reply_markup=None)
        await callback.answer()
        return

    if callback.message:
        await callback.message.edit_text(
            get("download.queue_title", lang),
            reply_markup=build_active_downloads_keyboard(lang, active),
        )
    await callback.answer()

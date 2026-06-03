from aiogram import Router, F
from aiogram.types import CallbackQuery
from aiogram.exceptions import TelegramBadRequest

from workers.tasks import download_and_send_task

callback_router = Router()


@callback_router.callback_query(F.data.startswith("fmt|"))
async def format_selected(callback: CallbackQuery, lang: str = "en"):
    await callback.answer()

    try:
        _, url_hash, format_id, platform = callback.data.split("|")
    except ValueError:
        return

    try:
        await callback.message.edit_reply_markup(reply_markup=None)
    except TelegramBadRequest:
        pass

    from bot.locale import get
    await callback.message.edit_text(get("download.downloading", lang))

    download_and_send_task.delay(
        url_hash=url_hash,
        format_id=format_id,
        platform=platform,
        chat_id=callback.message.chat.id,
        user_id=callback.from_user.id,
        message_id=callback.message.message_id,
        lang=lang,
    )


@callback_router.callback_query(F.data.startswith("cancel|"))
async def format_cancelled(callback: CallbackQuery, lang: str = "en"):
    await callback.answer()

    from bot.locale import get
    try:
        await callback.message.edit_text(
            get("download.cancelled", lang),
            reply_markup=None,
        )
    except TelegramBadRequest:
        pass

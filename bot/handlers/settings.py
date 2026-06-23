from aiogram import F, Router
from aiogram.filters import Command
from aiogram.fsm.context import FSMContext
from aiogram.types import CallbackQuery, InlineKeyboardButton, InlineKeyboardMarkup, Message

from bot.locale import get
from bot.services.user_settings import (
    get_or_create_user_settings,
    reset_user_settings,
    set_user_language,
    set_youtube_quality,
    set_youtube_ratio,
)

settings_router = Router()

_QUALITY_LABEL_KEYS = {
    "ask": "settings.quality_ask",
    "1080": "settings.quality_1080",
    "720": "settings.quality_720",
    "480": "settings.quality_480",
}

_RATIO_LABEL_KEYS = {
    "ask": "settings.ratio_ask",
    "16_9": "settings.ratio_16_9",
    "21_9": "settings.ratio_21_9",
    "9_16": "settings.ratio_9_16",
}


def _settings_keyboard(lang: str) -> InlineKeyboardMarkup:
    return InlineKeyboardMarkup(inline_keyboard=[
        [
            InlineKeyboardButton(
                text=get("settings.btn_lang", lang),
                callback_data="settings|lang",
            ),
        ],
        [
            InlineKeyboardButton(
                text=get("settings.btn_quality", lang),
                callback_data="settings|quality",
            ),
        ],
        [
            InlineKeyboardButton(
                text=get("settings.btn_ratio", lang),
                callback_data="settings|ratio",
            ),
        ],
        [
            InlineKeyboardButton(
                text=get("settings.btn_reset", lang),
                callback_data="settings|reset",
            ),
        ],
    ])


def _lang_keyboard(lang: str) -> InlineKeyboardMarkup:
    return InlineKeyboardMarkup(inline_keyboard=[
        [
            InlineKeyboardButton(
                text=get("onboarding.btn_en", lang),
                callback_data="settings|lang|en",
            ),
            InlineKeyboardButton(
                text=get("onboarding.btn_ru", lang),
                callback_data="settings|lang|ru",
            ),
        ],
        [
            InlineKeyboardButton(
                text=get("settings.btn_back", lang),
                callback_data="settings|menu",
            ),
        ],
    ])


def _quality_keyboard(lang: str) -> InlineKeyboardMarkup:
    return InlineKeyboardMarkup(inline_keyboard=[
        [
            InlineKeyboardButton(
                text=get("settings.quality_ask", lang),
                callback_data="settings|quality|ask",
            ),
        ],
        [
            InlineKeyboardButton(
                text=get("settings.quality_1080", lang),
                callback_data="settings|quality|1080",
            ),
            InlineKeyboardButton(
                text=get("settings.quality_720", lang),
                callback_data="settings|quality|720",
            ),
        ],
        [
            InlineKeyboardButton(
                text=get("settings.quality_480", lang),
                callback_data="settings|quality|480",
            ),
        ],
        [
            InlineKeyboardButton(
                text=get("settings.btn_back", lang),
                callback_data="settings|menu",
            ),
        ],
    ])


def _ratio_keyboard(lang: str) -> InlineKeyboardMarkup:
    return InlineKeyboardMarkup(inline_keyboard=[
        [
            InlineKeyboardButton(
                text=get("settings.ratio_ask", lang),
                callback_data="settings|ratio|ask",
            ),
        ],
        [
            InlineKeyboardButton(
                text=get("settings.ratio_16_9", lang),
                callback_data="settings|ratio|16_9",
            ),
            InlineKeyboardButton(
                text=get("settings.ratio_21_9", lang),
                callback_data="settings|ratio|21_9",
            ),
        ],
        [
            InlineKeyboardButton(
                text=get("settings.ratio_9_16", lang),
                callback_data="settings|ratio|9_16",
            ),
        ],
        [
            InlineKeyboardButton(
                text=get("settings.btn_back", lang),
                callback_data="settings|menu",
            ),
        ],
    ])


async def _settings_text(user_id: int, lang: str) -> str:
    settings = await get_or_create_user_settings(user_id)
    lang_label = get("settings.lang_en", lang) if lang == "en" else get("settings.lang_ru", lang)
    quality_label = get(_QUALITY_LABEL_KEYS.get(settings.youtube_quality, "settings.quality_ask"), lang)
    ratio_label = get(_RATIO_LABEL_KEYS.get(settings.youtube_ratio, "settings.ratio_ask"), lang)
    return (
        get("settings.title", lang) + "\n"
        + get("settings.lang_line", lang, language=lang_label) + "\n"
        + get("settings.quality_line", lang, quality=quality_label) + "\n"
        + get("settings.ratio_line", lang, ratio=ratio_label)
    )


@settings_router.message(Command("settings"))
async def cmd_settings(message: Message, state: FSMContext | None = None, lang: str = "en"):
    if state is not None:
        await state.clear()
    user_id = message.from_user.id if message.from_user else None
    if not user_id:
        return
    text = await _settings_text(user_id, lang)
    await message.answer(text, reply_markup=_settings_keyboard(lang))


@settings_router.callback_query(F.data == "settings|menu")
async def settings_menu(callback: CallbackQuery, state: FSMContext | None = None, lang: str = "en"):
    if state is not None:
        await state.clear()
    user_id = callback.from_user.id if callback.from_user else None
    if not user_id or not callback.message:
        await callback.answer()
        return
    text = await _settings_text(user_id, lang)
    await callback.message.edit_text(text, reply_markup=_settings_keyboard(lang))
    await callback.answer()


@settings_router.callback_query(F.data == "settings|lang")
async def settings_lang_menu(callback: CallbackQuery, lang: str = "en"):
    if not callback.message:
        await callback.answer()
        return
    await callback.message.edit_text(
        get("settings.lang_prompt", lang),
        reply_markup=_lang_keyboard(lang),
    )
    await callback.answer()


@settings_router.callback_query(F.data.startswith("settings|lang|"))
async def settings_lang_choose(callback: CallbackQuery, lang: str = "en"):
    user_id = callback.from_user.id if callback.from_user else None
    if not user_id or not callback.message:
        await callback.answer()
        return
    lang_code = callback.data.split("|")[2]
    if lang_code not in ("en", "ru"):
        await callback.answer()
        return
    await set_user_language(user_id, lang_code)
    text = await _settings_text(user_id, lang_code)
    await callback.message.edit_text(text, reply_markup=_settings_keyboard(lang_code))
    await callback.answer()


@settings_router.callback_query(F.data == "settings|quality")
async def settings_quality_menu(callback: CallbackQuery, lang: str = "en"):
    if not callback.message:
        await callback.answer()
        return
    await callback.message.edit_text(
        get("settings.quality_prompt", lang),
        reply_markup=_quality_keyboard(lang),
    )
    await callback.answer()


@settings_router.callback_query(F.data.startswith("settings|quality|"))
async def settings_quality_choose(callback: CallbackQuery, lang: str = "en"):
    user_id = callback.from_user.id if callback.from_user else None
    if not user_id or not callback.message:
        await callback.answer()
        return
    quality = callback.data.split("|")[2]
    if quality not in ("ask", "1080", "720", "480"):
        await callback.answer()
        return
    await set_youtube_quality(user_id, quality)
    text = await _settings_text(user_id, lang)
    await callback.message.edit_text(text, reply_markup=_settings_keyboard(lang))
    await callback.answer()


@settings_router.callback_query(F.data == "settings|ratio")
async def settings_ratio_menu(callback: CallbackQuery, lang: str = "en"):
    if not callback.message:
        await callback.answer()
        return
    await callback.message.edit_text(
        get("settings.ratio_prompt", lang),
        reply_markup=_ratio_keyboard(lang),
    )
    await callback.answer()


@settings_router.callback_query(F.data.startswith("settings|ratio|"))
async def settings_ratio_choose(callback: CallbackQuery, lang: str = "en"):
    user_id = callback.from_user.id if callback.from_user else None
    if not user_id or not callback.message:
        await callback.answer()
        return
    ratio = callback.data.split("|")[2]
    if ratio not in ("ask", "16_9", "21_9", "9_16"):
        await callback.answer()
        return
    await set_youtube_ratio(user_id, ratio)
    text = await _settings_text(user_id, lang)
    await callback.message.edit_text(text, reply_markup=_settings_keyboard(lang))
    await callback.answer()


@settings_router.callback_query(F.data == "settings|reset")
async def settings_reset(callback: CallbackQuery, lang: str = "en"):
    user_id = callback.from_user.id if callback.from_user else None
    if not user_id or not callback.message:
        await callback.answer()
        return
    await reset_user_settings(user_id)
    text = await _settings_text(user_id, lang)
    await callback.message.edit_text(text, reply_markup=_settings_keyboard(lang))
    await callback.answer(get("settings.reset_toast", lang))

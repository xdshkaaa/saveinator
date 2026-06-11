from aiogram import F, Router
from aiogram.filters import Command
from aiogram.fsm.context import FSMContext
from aiogram.fsm.state import State, StatesGroup
from aiogram.types import (
    CallbackQuery,
    InlineKeyboardButton,
    InlineKeyboardMarkup,
    Message,
)

from bot.config import settings
from bot.locale import get
from bot.metrics import record_command
from bot.services.runtime_settings import (
    SERVICE_ORDER,
    current_value,
    reset_runtime,
    service_settings,
    set_runtime_int,
    setting_definition,
)

admin_router = Router()
admin_router.message.filter(F.from_user.id == settings.admin_telegram_id)
admin_router.callback_query.filter(F.from_user.id == settings.admin_telegram_id)


class AdminEdit(StatesGroup):
    waiting_value = State()


SERVICE_LABELS = {
    "youtube": ("YouTube", "YouTube"),
    "tiktok": ("TikTok", "TikTok"),
    "instagram": ("Instagram", "Instagram"),
    "x": ("X / Twitter", "X / Twitter"),
    "spotify": ("Spotify", "Spotify"),
    "pinterest": ("Pinterest", "Pinterest"),
    "global": ("Global", "Глобально"),
}

KIND_LABELS = {
    "max_file_mb": ("Max file (MB)", "Лимит файла (МБ)"),
    "timeout_sec": ("Timeout (sec)", "Таймаут (сек)"),
}


def _service_label(service: str, lang: str) -> str:
    labels = SERVICE_LABELS.get(service, (service, service))
    return labels[1] if lang == "ru" else labels[0]


def _kind_label(kind: str, lang: str) -> str:
    labels = KIND_LABELS.get(kind, (kind, kind))
    return labels[1] if lang == "ru" else labels[0]


def _main_keyboard(lang: str) -> InlineKeyboardMarkup:
    rows = [
        [InlineKeyboardButton(
            text=_service_label(service, lang),
            callback_data=f"admin|svc|{service}",
        )]
        for service in SERVICE_ORDER
    ]
    rows.append([InlineKeyboardButton(
        text=get("admin.btn_global", lang),
        callback_data="admin|svc|global",
    )])
    rows.append([InlineKeyboardButton(
        text=get("admin.btn_reset_all", lang),
        callback_data="admin|reset|all",
    )])
    return InlineKeyboardMarkup(inline_keyboard=rows)


def _service_keyboard(service: str, lang: str) -> InlineKeyboardMarkup:
    rows: list[list[InlineKeyboardButton]] = []
    for defn in service_settings(service):
        current = get("admin.btn_edit", lang, kind=_kind_label(defn.kind, lang))
        rows.append([InlineKeyboardButton(
            text=current,
            callback_data=f"admin|edit|{defn.redis_key}",
        )])
    rows.append([InlineKeyboardButton(
        text=get("admin.btn_reset_service", lang),
        callback_data=f"admin|reset|svc|{service}",
    )])
    rows.append([InlineKeyboardButton(
        text=get("admin.btn_back", lang),
        callback_data="admin|menu",
    )])
    return InlineKeyboardMarkup(inline_keyboard=rows)


async def _service_summary(service: str, lang: str) -> str:
    lines = [get("admin.service_title", lang, service=_service_label(service, lang))]
    for defn in service_settings(service):
        value = await current_value(defn)
        lines.append(
            get(
                "admin.setting_line",
                lang,
                label=_kind_label(defn.kind, lang),
                value=value,
            )
        )
    lines.append(get("admin.hot_swap_hint", lang))
    return "\n".join(lines)


@admin_router.message(Command("admin"))
async def cmd_admin(message: Message, state: FSMContext, lang: str = "en"):
    record_command("admin")
    await state.clear()
    await message.answer(
        get("admin.menu_title", lang),
        reply_markup=_main_keyboard(lang),
    )


@admin_router.callback_query(F.data == "admin|menu")
async def admin_menu(callback: CallbackQuery, state: FSMContext, lang: str = "en"):
    await state.clear()
    await callback.message.edit_text(
        get("admin.menu_title", lang),
        reply_markup=_main_keyboard(lang),
    )
    await callback.answer()


@admin_router.callback_query(F.data.startswith("admin|svc|"))
async def admin_service(callback: CallbackQuery, lang: str = "en"):
    service = callback.data.split("|", 2)[2]
    if service not in SERVICE_ORDER and service != "global":
        await callback.answer()
        return
    await callback.message.edit_text(
        await _service_summary(service, lang),
        reply_markup=_service_keyboard(service, lang),
    )
    await callback.answer()


@admin_router.callback_query(F.data.startswith("admin|edit|"))
async def admin_edit_start(callback: CallbackQuery, state: FSMContext, lang: str = "en"):
    redis_key = callback.data.split("|", 2)[2]
    defn = setting_definition(redis_key)
    if defn is None:
        await callback.answer()
        return

    await state.set_state(AdminEdit.waiting_value)
    await state.update_data(redis_key=redis_key, service=defn.service)
    current = await current_value(defn)
    await callback.message.edit_text(
        get(
            "admin.enter_value",
            lang,
            service=_service_label(defn.service, lang),
            label=_kind_label(defn.kind, lang),
            current=current,
        ),
        reply_markup=InlineKeyboardMarkup(inline_keyboard=[[
            InlineKeyboardButton(
                text=get("admin.btn_back", lang),
                callback_data=f"admin|svc|{defn.service}",
            )
        ]]),
    )
    await callback.answer()


@admin_router.message(AdminEdit.waiting_value)
async def admin_edit_value(message: Message, state: FSMContext, lang: str = "en"):
    data = await state.get_data()
    redis_key = data.get("redis_key")
    service = data.get("service", "youtube")
    defn = setting_definition(redis_key) if redis_key else None
    if defn is None:
        await state.clear()
        return

    raw = (message.text or "").strip()
    try:
        value = int(raw)
    except ValueError:
        await message.reply(get("admin.invalid_number", lang))
        return

    if value <= 0:
        await message.reply(get("admin.invalid_number", lang))
        return

    await set_runtime_int(redis_key, value)
    await state.clear()
    await message.answer(
        get(
            "admin.saved",
            lang,
            label=_kind_label(defn.kind, lang),
            value=value,
        ),
        reply_markup=_service_keyboard(service, lang),
    )
    await message.answer(await _service_summary(service, lang))


@admin_router.callback_query(F.data.startswith("admin|reset|"))
async def admin_reset(callback: CallbackQuery, state: FSMContext, lang: str = "en"):
    await state.clear()
    parts = callback.data.split("|")
    if len(parts) == 3 and parts[2] == "all":
        await reset_runtime(None)
        await callback.message.edit_text(
            get("admin.reset_all_done", lang),
            reply_markup=_main_keyboard(lang),
        )
        await callback.answer(get("admin.reset_done_toast", lang))
        return

    if len(parts) == 4 and parts[2] == "svc":
        service = parts[3]
        for defn in service_settings(service):
            await reset_runtime(defn.redis_key)
        await callback.message.edit_text(
            await _service_summary(service, lang),
            reply_markup=_service_keyboard(service, lang),
        )
        await callback.answer(get("admin.reset_done_toast", lang))
        return

    await callback.answer()

import structlog
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

from bot.filters.admin import IsAdminFilter
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
from bot.services.user_bans import ban_user, list_banned_users, unban_user

logger = structlog.get_logger()
admin_router = Router()
_is_admin = IsAdminFilter()


class AdminEdit(StatesGroup):
    waiting_value = State()


class AdminBan(StatesGroup):
    waiting_ban_id = State()


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
        text=get("admin.btn_bans", lang),
        callback_data="admin|bans",
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


async def _bans_summary(lang: str) -> str:
    banned_ids = await list_banned_users()
    lines = [get("admin.bans_title", lang)]
    if not banned_ids:
        lines.append(get("admin.bans_empty", lang))
    else:
        for user_id in banned_ids:
            lines.append(get("admin.ban_user_line", lang, user_id=user_id))
    lines.append(get("admin.bans_hint", lang))
    return "\n".join(lines)


def _bans_keyboard(lang: str, banned_ids: list[int]) -> InlineKeyboardMarkup:
    rows: list[list[InlineKeyboardButton]] = []
    for user_id in banned_ids:
        rows.append([InlineKeyboardButton(
            text=get("admin.btn_unban_user", lang, user_id=user_id),
            callback_data=f"admin|unban|{user_id}",
        )])
    rows.append([InlineKeyboardButton(
        text=get("admin.btn_ban_add", lang),
        callback_data="admin|ban|add",
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


@admin_router.message(Command("admin"), _is_admin)
async def cmd_admin(message: Message, state: FSMContext, lang: str = "en"):
    record_command("admin")
    logger.info("admin panel opened", user_id=message.from_user.id if message.from_user else None)
    await state.clear()
    await message.answer(
        get("admin.menu_title", lang),
        reply_markup=_main_keyboard(lang),
    )


@admin_router.callback_query(F.data == "admin|menu", _is_admin)
async def admin_menu(callback: CallbackQuery, state: FSMContext, lang: str = "en"):
    await state.clear()
    await callback.message.edit_text(
        get("admin.menu_title", lang),
        reply_markup=_main_keyboard(lang),
    )
    await callback.answer()


@admin_router.callback_query(F.data.startswith("admin|svc|"), _is_admin)
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


@admin_router.callback_query(F.data.startswith("admin|edit|"), _is_admin)
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


@admin_router.message(AdminEdit.waiting_value, _is_admin)
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
    logger.info("admin setting updated", key=redis_key, value=value)
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


@admin_router.callback_query(F.data == "admin|bans", _is_admin)
async def admin_bans_menu(callback: CallbackQuery, state: FSMContext, lang: str = "en"):
    await state.clear()
    banned_ids = await list_banned_users()
    await callback.message.edit_text(
        await _bans_summary(lang),
        reply_markup=_bans_keyboard(lang, banned_ids),
    )
    await callback.answer()


@admin_router.callback_query(F.data == "admin|ban|add", _is_admin)
async def admin_ban_add_start(callback: CallbackQuery, state: FSMContext, lang: str = "en"):
    await state.set_state(AdminBan.waiting_ban_id)
    await callback.message.edit_text(
        get("admin.enter_ban_id", lang),
        reply_markup=InlineKeyboardMarkup(inline_keyboard=[[
            InlineKeyboardButton(
                text=get("admin.btn_back", lang),
                callback_data="admin|bans",
            )
        ]]),
    )
    await callback.answer()


@admin_router.message(AdminBan.waiting_ban_id, _is_admin)
async def admin_ban_add_value(message: Message, state: FSMContext, lang: str = "en"):
    raw = (message.text or "").strip()
    try:
        user_id = int(raw)
    except ValueError:
        await message.reply(get("admin.invalid_user_id", lang))
        return

    if user_id <= 0:
        await message.reply(get("admin.invalid_user_id", lang))
        return

    if user_id == message.from_user.id if message.from_user else None:
        await message.reply(get("admin.cannot_ban_self", lang))
        return

    await ban_user(user_id)
    await state.clear()
    banned_ids = await list_banned_users()
    await message.answer(
        get("admin.ban_added", lang, user_id=user_id),
        reply_markup=_bans_keyboard(lang, banned_ids),
    )
    await message.answer(await _bans_summary(lang))


@admin_router.callback_query(F.data.startswith("admin|unban|"), _is_admin)
async def admin_unban_user(callback: CallbackQuery, state: FSMContext, lang: str = "en"):
    await state.clear()
    user_id_raw = callback.data.split("|", 2)[2]
    try:
        user_id = int(user_id_raw)
    except ValueError:
        await callback.answer()
        return

    removed = await unban_user(user_id)
    banned_ids = await list_banned_users()
    toast = (
        get("admin.unban_done", lang, user_id=user_id)
        if removed
        else get("admin.unban_not_found", lang, user_id=user_id)
    )
    await callback.message.edit_text(
        await _bans_summary(lang),
        reply_markup=_bans_keyboard(lang, banned_ids),
    )
    await callback.answer(toast)


@admin_router.callback_query(F.data.startswith("admin|reset|"), _is_admin)
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

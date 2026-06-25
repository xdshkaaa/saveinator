"""Admin panel — runtime settings, bans, and broadcasts menu.

Callback data protocol
----------------------
``admin|<action>|<param>``

Actions
~~~~~~~
- ``menu`` — main admin menu
- ``svc|<service>`` — show service settings page
- ``edit|<redis_key>`` — start editing a setting (FSM)
- ``edit_bool|<redis_key>|<value>`` — set a bool setting directly
- ``edit_enum|<redis_key>|<value>`` — set an enum setting directly
- ``bans`` — bans management menu
- ``stats`` — user statistics screen
- ``stats|refresh`` — refresh user statistics
- ``ban|add`` — add ban (FSM)
- ``unban|<user_id>`` — unban a user
- ``reset|all`` — reset all runtime overrides
- ``reset|svc|<service>`` — reset a single service
- ``confirm|reset_all`` — confirm reset all dialog
- ``confirm|reset_svc|<service>`` — confirm reset service dialog
- ``confirm|reset_all_yes`` — execute reset all
- ``confirm|reset_svc_yes|<service>`` — execute reset service
- ``confirm|disable|<service>`` — confirm disable platform
- ``confirm|disable_yes|<service>`` — execute disable
- ``broadcasts`` — broadcasts menu
"""

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
from bot.metrics import record_command, ADMIN_RUNTIME_SETTINGS_CHANGED_TOTAL
from bot.services.admin_log import log_setting_change
from bot.services.runtime_settings import (
    SERVICE_ORDER,
    current_value,
    format_value,
    kind_label,
    reset_runtime,
    service_label,
    service_settings,
    set_runtime_value,
    setting_definition,
    validate_value,
)
from bot.services.user_bans import ban_user, list_banned_users, unban_user
from bot.services.user_stats import UserStatsSnapshot, fetch_user_stats

logger = structlog.get_logger()
admin_router = Router()
_is_admin = IsAdminFilter()


# ---------------------------------------------------------------------------
# FSM
# ---------------------------------------------------------------------------


class AdminEdit(StatesGroup):
    waiting_value = State()


class AdminBan(StatesGroup):
    waiting_ban_id = State()


# ---------------------------------------------------------------------------
# Keyboard builders
# ---------------------------------------------------------------------------


def _main_keyboard(lang: str) -> InlineKeyboardMarkup:
    rows = [
        [InlineKeyboardButton(
            text=service_label(service, lang),
            callback_data=f"admin|svc|{service}",
        )]
        for service in SERVICE_ORDER
    ]
    rows.append([InlineKeyboardButton(
        text=get("admin.btn_global", lang),
        callback_data="admin|svc|global",
    )])
    rows.append([InlineKeyboardButton(
        text=get("admin.btn_broadcasts", lang),
        callback_data="admin|broadcasts",
    )])
    rows.append([InlineKeyboardButton(
        text=get("admin.btn_users", lang),
        callback_data="admin|stats",
    )])
    rows.append([InlineKeyboardButton(
        text=get("admin.btn_bans", lang),
        callback_data="admin|bans",
    )])
    rows.append([InlineKeyboardButton(
        text=get("admin.btn_reset_all", lang),
        callback_data="admin|confirm|reset_all",
    )])
    return InlineKeyboardMarkup(inline_keyboard=rows)


def _service_keyboard(service: str, lang: str) -> InlineKeyboardMarkup:
    rows: list[list[InlineKeyboardButton]] = []
    for defn in service_settings(service):
        label = kind_label(defn, lang)
        if defn.value_type == "bool":
            rows.append([InlineKeyboardButton(
                text=get("admin.btn_change", lang, label=label),
                callback_data=f"admin|edit_bool|{defn.redis_key}",
            )])
        elif defn.value_type == "enum":
            rows.append([InlineKeyboardButton(
                text=get("admin.btn_change", lang, label=label),
                callback_data=f"admin|edit|{defn.redis_key}",
            )])
        else:
            rows.append([InlineKeyboardButton(
                text=get("admin.btn_change", lang, label=label),
                callback_data=f"admin|edit|{defn.redis_key}",
            )])
    rows.append([InlineKeyboardButton(
        text=get("admin.btn_reset_service", lang),
        callback_data=f"admin|confirm|reset_svc|{service}",
    )])
    rows.append([InlineKeyboardButton(
        text=get("admin.btn_back", lang),
        callback_data="admin|menu",
    )])
    return InlineKeyboardMarkup(inline_keyboard=rows)


async def _service_summary(service: str, lang: str) -> str:
    lines = [get("admin.service_title", lang, service=service_label(service, lang))]
    settings = service_settings(service)
    if not settings:
        lines.append(get("admin.no_settings", lang))
    else:
        for defn in settings:
            value = await current_value(defn)
            label = kind_label(defn, lang)
            display = format_value(value, defn, lang)
            lines.append(get("admin.setting_line", lang, label=label, value=display))
    lines.append(get("admin.hot_swap_hint", lang))
    return "\n".join(lines)


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


def _stats_keyboard(lang: str) -> InlineKeyboardMarkup:
    return InlineKeyboardMarkup(inline_keyboard=[
        [
            InlineKeyboardButton(
                text=get("admin.btn_stats_refresh", lang),
                callback_data="admin|stats|refresh",
            ),
            InlineKeyboardButton(
                text=get("admin.btn_back", lang),
                callback_data="admin|menu",
            ),
        ],
    ])


def _format_growth_delta(snapshot: UserStatsSnapshot, lang: str) -> str:
    diff = snapshot.new_today - snapshot.new_yesterday
    if diff == 0 and snapshot.new_today == 0:
        return get("admin.stats_growth_delta_flat", lang)
    if snapshot.new_yesterday == 0:
        pct = "—" if diff == 0 else "new"
    else:
        pct = f"{round(100 * diff / snapshot.new_yesterday):+d}%"
    return get("admin.stats_growth_delta", lang, diff=f"{diff:+d}", pct=pct)


def _format_stats(snapshot: UserStatsSnapshot, lang: str) -> str:
    if snapshot.top_platforms_7d:
        platform_lines = "\n".join(
            get("admin.stats_platform_line", lang, platform=platform, count=count)
            for platform, count in snapshot.top_platforms_7d
        )
    else:
        platform_lines = get("admin.stats_platforms_empty", lang)

    body = get(
        "admin.stats_body",
        lang,
        total=snapshot.total_users,
        new_today=snapshot.new_today,
        growth_delta=_format_growth_delta(snapshot, lang),
        new_7d=snapshot.new_7d,
        new_30d=snapshot.new_30d,
        active_now=snapshot.active_now,
        dau=snapshot.dau,
        wau=snapshot.wau,
        mau=snapshot.mau,
        with_downloads=snapshot.users_with_downloads,
        returning=snapshot.returning_users,
        lang_en=snapshot.language_en,
        lang_ru=snapshot.language_ru,
        platform_lines=platform_lines,
        banned=snapshot.banned_count,
        active_note=get("admin.stats_active_note", lang),
        download_note=get("admin.stats_download_note", lang),
    )
    return f"{get('admin.stats_title', lang)}\n\n{body}"


async def _render_stats(lang: str) -> str:
    snapshot = await fetch_user_stats()
    return _format_stats(snapshot, lang)


# ---------------------------------------------------------------------------
# /admin command
# ---------------------------------------------------------------------------


@admin_router.message(Command("admin"), _is_admin)
async def cmd_admin(message: Message, state: FSMContext, lang: str = "en"):
    record_command("admin")
    logger.info("admin panel opened", user_id=message.from_user.id if message.from_user else None)
    await state.clear()
    await message.answer(
        get("admin.menu_title", lang),
        reply_markup=_main_keyboard(lang),
    )


@admin_router.message(Command("stats"), _is_admin)
async def cmd_stats(message: Message, state: FSMContext, lang: str = "en"):
    record_command("stats")
    await state.clear()
    await message.answer(
        await _render_stats(lang),
        reply_markup=_stats_keyboard(lang),
    )


@admin_router.callback_query(F.data == "admin|menu", _is_admin)
async def admin_menu(callback: CallbackQuery, state: FSMContext, lang: str = "en"):
    await state.clear()
    await callback.message.edit_text(
        get("admin.menu_title", lang),
        reply_markup=_main_keyboard(lang),
    )
    await callback.answer()


# ---------------------------------------------------------------------------
# Service page
# ---------------------------------------------------------------------------


@admin_router.callback_query(F.data.startswith("admin|svc|"), _is_admin)
async def admin_service(callback: CallbackQuery, state: FSMContext, lang: str = "en"):
    await state.clear()
    service = callback.data.split("|", 2)[2]
    if service not in SERVICE_ORDER and service != "global":
        await callback.answer()
        return
    await callback.message.edit_text(
        await _service_summary(service, lang),
        reply_markup=_service_keyboard(service, lang),
    )
    await callback.answer()


# ---------------------------------------------------------------------------
# Edit value — text input (int, list, enum fallback)
# ---------------------------------------------------------------------------


@admin_router.callback_query(F.data.startswith("admin|edit|"), _is_admin)
async def admin_edit_start(callback: CallbackQuery, state: FSMContext, lang: str = "en"):
    redis_key = callback.data.split("|", 2)[2]
    defn = setting_definition(redis_key)
    if defn is None:
        await callback.answer()
        return

    await state.set_state(AdminEdit.waiting_value)
    await state.update_data(redis_key=redis_key, service=defn.service)

    label = kind_label(defn, lang)
    current = await current_value(defn)
    current_display = format_value(current, defn, lang)

    hint = ""
    if defn.value_type == "int" and (defn.min_value is not None or defn.max_value is not None):
        hint_parts = []
        if defn.min_value is not None:
            hint_parts.append(f"min={defn.min_value}")
        if defn.max_value is not None:
            hint_parts.append(f"max={defn.max_value}")
        hint = f"\n({', '.join(hint_parts)})"
    elif defn.value_type == "list" and defn.allowed_values:
        hint = f"\nAvailable: {', '.join(defn.allowed_values)}"

    await callback.message.edit_text(
        get("admin.enter_value", lang, service=service_label(defn.service, lang), label=label, current=current_display)
        + hint,
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
    error = validate_value(defn, raw)
    if error is not None:
        await message.reply(get("admin.invalid_value", lang, error=error))
        return

    # Parse the value based on type
    parsed = _parse_typed_value(raw, defn)

    old_value = await current_value(defn)
    await set_runtime_value(redis_key, parsed)

    admin_id = message.from_user.id if message.from_user else 0
    ADMIN_RUNTIME_SETTINGS_CHANGED_TOTAL.labels(service=defn.service).inc()
    log_setting_change(admin_id, redis_key, old_value, parsed)
    logger.info("admin setting updated", key=redis_key, value=parsed)

    await state.clear()
    label = kind_label(defn, lang)
    display = format_value(parsed, defn, lang)
    await message.answer(
        get("admin.saved", lang, label=label, value=display),
        reply_markup=_service_keyboard(service, lang),
    )
    await message.answer(await _service_summary(service, lang))


def _parse_typed_value(raw: str, defn) -> int | bool | tuple[str, ...] | str:
    """Parse a raw string into the correct type based on defn.value_type."""
    if defn.value_type == "bool":
        return raw.strip().lower() in ("1", "true", "yes", "on")
    if defn.value_type in ("enum", "list"):
        return tuple(v.strip() for v in raw.split(",") if v.strip())
    return int(raw)


# ---------------------------------------------------------------------------
# Edit bool — inline toggle
# ---------------------------------------------------------------------------


@admin_router.callback_query(F.data.startswith("admin|edit_bool|"), _is_admin)
async def admin_edit_bool(callback: CallbackQuery, lang: str = "en"):
    parts = callback.data.split("|", 2)
    redis_key = parts[2]
    defn = setting_definition(redis_key)
    if defn is None:
        await callback.answer()
        return

    old_value = await current_value(defn)
    new_value = not old_value

    await set_runtime_value(redis_key, new_value)

    admin_id = callback.from_user.id if callback.from_user else 0
    ADMIN_RUNTIME_SETTINGS_CHANGED_TOTAL.labels(service=defn.service).inc()
    log_setting_change(admin_id, redis_key, old_value, new_value)
    logger.info("admin setting updated", key=redis_key, value=new_value)

    label = kind_label(defn, lang)
    display = format_value(new_value, defn, lang)
    await callback.message.edit_text(
        await _service_summary(defn.service, lang),
        reply_markup=_service_keyboard(defn.service, lang),
    )
    await callback.answer(get("admin.saved", lang, label=label, value=display))


# ---------------------------------------------------------------------------
# Confirmation dialogs
# ---------------------------------------------------------------------------


@admin_router.callback_query(F.data == "admin|confirm|reset_all", _is_admin)
async def admin_confirm_reset_all(callback: CallbackQuery, lang: str = "en"):
    await callback.message.edit_text(
        get("admin.confirm_reset_all", lang),
        reply_markup=InlineKeyboardMarkup(inline_keyboard=[
            [
                InlineKeyboardButton(
                    text=get("admin.confirm_yes", lang),
                    callback_data="admin|confirm|reset_all_yes",
                ),
                InlineKeyboardButton(
                    text=get("admin.confirm_no", lang),
                    callback_data="admin|menu",
                ),
            ],
        ]),
    )
    await callback.answer()


@admin_router.callback_query(F.data == "admin|confirm|reset_all_yes", _is_admin)
async def admin_confirm_reset_all_yes(callback: CallbackQuery, lang: str = "en"):
    await reset_runtime(None)
    admin_id = callback.from_user.id if callback.from_user else 0
    log_setting_change(admin_id, "ALL", "all overrides", "defaults")
    logger.info("admin reset all settings", admin_id=admin_id)
    await callback.message.edit_text(
        get("admin.reset_all_done", lang),
        reply_markup=_main_keyboard(lang),
    )
    await callback.answer(get("admin.reset_done_toast", lang))


@admin_router.callback_query(F.data.startswith("admin|confirm|reset_svc|"), _is_admin)
async def admin_confirm_reset_svc(callback: CallbackQuery, lang: str = "en"):
    service = callback.data.split("|", 3)[3]
    await callback.message.edit_text(
        get("admin.confirm_reset_service", lang, service=service_label(service, lang)),
        reply_markup=InlineKeyboardMarkup(inline_keyboard=[
            [
                InlineKeyboardButton(
                    text=get("admin.confirm_yes", lang),
                    callback_data=f"admin|confirm|reset_svc_yes|{service}",
                ),
                InlineKeyboardButton(
                    text=get("admin.confirm_no", lang),
                    callback_data=f"admin|svc|{service}",
                ),
            ],
        ]),
    )
    await callback.answer()


@admin_router.callback_query(F.data.startswith("admin|confirm|reset_svc_yes|"), _is_admin)
async def admin_confirm_reset_svc_yes(callback: CallbackQuery, lang: str = "en"):
    service = callback.data.split("|", 3)[3]
    for defn in service_settings(service):
        await reset_runtime(defn.redis_key)
    admin_id = callback.from_user.id if callback.from_user else 0
    log_setting_change(admin_id, f"service:{service}", "all", "defaults")
    logger.info("admin reset service settings", admin_id=admin_id, service=service)
    await callback.message.edit_text(
        await _service_summary(service, lang),
        reply_markup=_service_keyboard(service, lang),
    )
    await callback.answer(get("admin.reset_done_toast", lang))


# ---------------------------------------------------------------------------
# User stats
# ---------------------------------------------------------------------------


@admin_router.callback_query(F.data.in_({"admin|stats", "admin|stats|refresh"}), _is_admin)
async def admin_stats(callback: CallbackQuery, state: FSMContext, lang: str = "en"):
    await state.clear()
    await callback.message.edit_text(
        await _render_stats(lang),
        reply_markup=_stats_keyboard(lang),
    )
    await callback.answer()


# ---------------------------------------------------------------------------
# Bans
# ---------------------------------------------------------------------------


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


# ---------------------------------------------------------------------------
# Reset — direct callbacks (non-confirm, used by legacy / tests)
# ---------------------------------------------------------------------------


@admin_router.callback_query(F.data.startswith("admin|reset|"), _is_admin)
async def admin_reset(callback: CallbackQuery, state: FSMContext, lang: str = "en"):
    """Direct reset without confirmation — kept for backward compat."""
    await state.clear()
    parts = callback.data.split("|")
    if len(parts) == 3 and parts[2] == "all":
        await reset_runtime(None)
        admin_id = callback.from_user.id if callback.from_user else 0
        log_setting_change(admin_id, "ALL", "all overrides", "defaults")
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
        admin_id = callback.from_user.id if callback.from_user else 0
        log_setting_change(admin_id, f"service:{service}", "all", "defaults")
        await callback.message.edit_text(
            await _service_summary(service, lang),
            reply_markup=_service_keyboard(service, lang),
        )
        await callback.answer(get("admin.reset_done_toast", lang))
        return

    await callback.answer()

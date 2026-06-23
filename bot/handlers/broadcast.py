"""Broadcast handler — create, preview, send, and track info broadcasts.

Callback data protocol
----------------------
``broadcast|<action>|<param>``

Actions
~~~~~~~
- ``menu`` — broadcast main menu
- ``create`` — start creating a new broadcast (FSM)
- ``history`` — show broadcast history
- ``status`` — show active broadcast status
- ``preview|<id>`` — show broadcast preview with audience selection
- ``audience|<id>|<audience>`` — select audience and show final preview
- ``send|<id>`` — confirm and queue broadcast for sending
- ``edit_text|<id>`` — go back to text editing (FSM)
- ``test|<id>`` — send test message to admin
- ``cancel|<id>`` — cancel a draft broadcast
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
from sqlalchemy import select, func

from bot.config import settings
from bot.filters.admin import IsAdminFilter
from bot.metrics import BROADCASTS_TOTAL
from bot.locale import get
from bot.services.admin_log import log_broadcast_action
from bot.services.broadcast_service import (
    BroadcastAudience,
    BroadcastStatus,
    audience_display_name,
    count_recipients,
    create_broadcast,
    get_active_broadcast,
    get_broadcast,
    get_broadcast_stats,
    get_broadcasts_history,
    get_recipient_ids,
    status_display_name,
    update_broadcast_status,
    update_broadcast_text,
)

logger = structlog.get_logger()
broadcast_router = Router()
_is_admin = IsAdminFilter()


# ---------------------------------------------------------------------------
# FSM
# ---------------------------------------------------------------------------


class BroadcastCreate(StatesGroup):
    waiting_text = State()
    waiting_edit_text = State()  # re-edit after preview


# ---------------------------------------------------------------------------
# Menu
# ---------------------------------------------------------------------------


def _menu_keyboard(lang: str) -> InlineKeyboardMarkup:
    return InlineKeyboardMarkup(inline_keyboard=[
        [InlineKeyboardButton(
            text=get("broadcast.btn_create", lang),
            callback_data="broadcast|create",
        )],
        [InlineKeyboardButton(
            text=get("broadcast.btn_history", lang),
            callback_data="broadcast|history",
        )],
        [InlineKeyboardButton(
            text=get("broadcast.btn_status", lang),
            callback_data="broadcast|status",
        )],
        [InlineKeyboardButton(
            text=get("broadcast.btn_back", lang),
            callback_data="admin|menu",
        )],
    ])


# ---------------------------------------------------------------------------
# Route from admin menu
# ---------------------------------------------------------------------------


@broadcast_router.callback_query(F.data == "admin|broadcasts", _is_admin)
@broadcast_router.callback_query(F.data == "broadcast|menu", _is_admin)
async def broadcast_menu(callback: CallbackQuery, state: FSMContext, lang: str = "en"):
    await state.clear()
    await callback.message.edit_text(
        get("broadcast.menu_title", lang),
        reply_markup=_menu_keyboard(lang),
    )
    await callback.answer()


@broadcast_router.message(Command("broadcast"), _is_admin)
async def cmd_broadcast(message: Message, state: FSMContext, lang: str = "en"):
    await state.clear()
    await message.answer(
        get("broadcast.menu_title", lang),
        reply_markup=_menu_keyboard(lang),
    )


# ---------------------------------------------------------------------------
# Create — step 1: enter text
# ---------------------------------------------------------------------------


@broadcast_router.callback_query(F.data == "broadcast|create", _is_admin)
async def broadcast_create_start(callback: CallbackQuery, state: FSMContext, lang: str = "en"):
    await state.set_state(BroadcastCreate.waiting_text)
    await callback.message.edit_text(
        get("broadcast.enter_text", lang),
        reply_markup=InlineKeyboardMarkup(inline_keyboard=[[
            InlineKeyboardButton(
                text=get("broadcast.btn_back", lang),
                callback_data="broadcast|menu",
            )
        ]]),
    )
    await callback.answer()


@broadcast_router.message(BroadcastCreate.waiting_text, _is_admin)
async def broadcast_create_text(message: Message, state: FSMContext, lang: str = "en"):
    text = (message.text or "").strip()
    if not text:
        await message.reply("Text cannot be empty.")
        return

    admin_id = message.from_user.id if message.from_user else 0
    broadcast = await create_broadcast(admin_id, text)
    await state.clear()

    # Show preview with audience selection
    await _show_audience_selection(message, broadcast.id, lang)


# ---------------------------------------------------------------------------
# Create — step 1.5: audience selection
# ---------------------------------------------------------------------------


async def _show_audience_selection(
    target: Message | CallbackQuery,
    broadcast_id: int,
    lang: str,
) -> None:
    text = get("broadcast.audience_prompt", lang)
    kb = InlineKeyboardMarkup(inline_keyboard=[
        [InlineKeyboardButton(
            text=get("broadcast.audience_all", lang),
            callback_data=f"broadcast|audience|{broadcast_id}|all",
        )],
        [InlineKeyboardButton(
            text=get("broadcast.audience_ru", lang),
            callback_data=f"broadcast|audience|{broadcast_id}|ru",
        )],
        [InlineKeyboardButton(
            text=get("broadcast.audience_en", lang),
            callback_data=f"broadcast|audience|{broadcast_id}|en",
        )],
        [InlineKeyboardButton(
            text=get("broadcast.audience_active", lang),
            callback_data=f"broadcast|audience|{broadcast_id}|active",
        )],
        [InlineKeyboardButton(
            text=get("broadcast.audience_test", lang),
            callback_data=f"broadcast|audience|{broadcast_id}|test",
        )],
        [InlineKeyboardButton(
            text=get("broadcast.btn_back", lang),
            callback_data="broadcast|menu",
        )],
    ])

    if isinstance(target, Message):
        await target.answer(text, reply_markup=kb)
    else:
        await target.message.edit_text(text, reply_markup=kb)


# ---------------------------------------------------------------------------
# Create — step 2: audience selected → show final preview
# ---------------------------------------------------------------------------


@broadcast_router.callback_query(F.data.startswith("broadcast|audience|"), _is_admin)
async def broadcast_audience_selected(callback: CallbackQuery, lang: str = "en"):
    parts = callback.data.split("|")
    broadcast_id = int(parts[2])
    audience_raw = parts[3]

    if audience_raw == "test":
        # Test send to admin only
        broadcast = await get_broadcast(broadcast_id)
        if broadcast is None:
            await callback.answer("Broadcast not found.")
            return

        admin_id = callback.from_user.id if callback.from_user else 0
        try:
            from aiogram import Bot
            from bot.dispatcher import _get_bot

            # Try to get bot from the callback context
            bot = callback.bot
            await bot.send_message(admin_id, broadcast.text)
            await callback.answer(get("broadcast.test_sent", lang))
        except Exception as e:
            logger.error("test broadcast send failed", error=e)
            await callback.answer("Failed to send test message.")
        return

    audience = BroadcastAudience(audience_raw)
    broadcast = await get_broadcast(broadcast_id)
    if broadcast is None:
        await callback.answer("Broadcast not found.")
        return

    await update_broadcast_text(broadcast_id, broadcast.text)

    total = await count_recipients(audience)

    preview_text = (
        get("broadcast.preview_title", lang, text=broadcast.text)
        + "\n\n"
        + get("broadcast.preview_audience", lang, audience=audience_display_name(audience, lang))
        + "\n"
        + get("broadcast.preview_recipients", lang, count=total)
        + "\n\n"
        + get("broadcast.preview_confirm", lang)
    )

    kb = InlineKeyboardMarkup(inline_keyboard=[
        [
            InlineKeyboardButton(
                text=get("broadcast.btn_send", lang),
                callback_data=f"broadcast|send|{broadcast_id}|{audience_raw}",
            ),
        ],
        [
            InlineKeyboardButton(
                text=get("broadcast.btn_edit_text", lang),
                callback_data=f"broadcast|edit_text|{broadcast_id}",
            ),
        ],
        [
            InlineKeyboardButton(
                text=get("broadcast.btn_test", lang),
                callback_data=f"broadcast|audience|{broadcast_id}|test",
            ),
        ],
        [
            InlineKeyboardButton(
                text=get("broadcast.btn_cancel", lang),
                callback_data=f"broadcast|cancel|{broadcast_id}",
            ),
        ],
    ])

    await callback.message.edit_text(preview_text, reply_markup=kb)
    await callback.answer()


# ---------------------------------------------------------------------------
# Edit text
# ---------------------------------------------------------------------------


@broadcast_router.callback_query(F.data.startswith("broadcast|edit_text|"), _is_admin)
async def broadcast_edit_text(callback: CallbackQuery, state: FSMContext, lang: str = "en"):
    broadcast_id = int(callback.data.split("|")[2])
    await state.set_state(BroadcastCreate.waiting_edit_text)
    await state.update_data(broadcast_id=broadcast_id)
    await callback.message.edit_text(
        get("broadcast.enter_text", lang),
        reply_markup=InlineKeyboardMarkup(inline_keyboard=[[
            InlineKeyboardButton(
                text=get("broadcast.btn_back", lang),
                callback_data="broadcast|menu",
            )
        ]]),
    )
    await callback.answer()


@broadcast_router.message(BroadcastCreate.waiting_edit_text, _is_admin)
async def broadcast_edit_text_save(message: Message, state: FSMContext, lang: str = "en"):
    data = await state.get_data()
    broadcast_id = data.get("broadcast_id")
    if broadcast_id is None:
        await state.clear()
        return

    text = (message.text or "").strip()
    if not text:
        await message.reply("Text cannot be empty.")
        return

    await update_broadcast_text(broadcast_id, text)
    await state.clear()
    await _show_audience_selection(message, broadcast_id, lang)


# ---------------------------------------------------------------------------
# Cancel draft
# ---------------------------------------------------------------------------


@broadcast_router.callback_query(F.data.startswith("broadcast|cancel|"), _is_admin)
async def broadcast_cancel(callback: CallbackQuery, state: FSMContext, lang: str = "en"):
    await state.clear()
    broadcast_id = int(callback.data.split("|")[2])
    await update_broadcast_status(broadcast_id, BroadcastStatus.CANCELLED)
    admin_id = callback.from_user.id if callback.from_user else 0
    log_broadcast_action(admin_id, broadcast_id, "cancelled")
    await callback.message.edit_text(
        get("broadcast.cancelled", lang),
        reply_markup=_menu_keyboard(lang),
    )
    await callback.answer()


# ---------------------------------------------------------------------------
# Confirm & queue send
# ---------------------------------------------------------------------------


@broadcast_router.callback_query(F.data.startswith("broadcast|send|"), _is_admin)
async def broadcast_send(callback: CallbackQuery, state: FSMContext, lang: str = "en"):
    await state.clear()
    parts = callback.data.split("|")
    broadcast_id = int(parts[2])
    audience_raw = parts[3]
    audience = BroadcastAudience(audience_raw)

    broadcast = await get_broadcast(broadcast_id)
    if broadcast is None:
        await callback.answer("Broadcast not found.")
        return

    # Get recipients
    if audience_raw == "test":
        user_ids = [callback.from_user.id]
    else:
        user_ids = await get_recipient_ids(audience)

    total = len(user_ids)
    await update_broadcast_status(
        broadcast_id,
        BroadcastStatus.QUEUED,
        total_recipients=total,
    )

    BROADCASTS_TOTAL.labels(audience=audience_raw).inc()
    admin_id = callback.from_user.id if callback.from_user else 0
    log_broadcast_action(admin_id, broadcast_id, "queued", audience=audience_raw, recipients=total)

    # Enqueue Celery task
    try:
        from workers.broadcast_task import execute_broadcast
        execute_broadcast.delay(broadcast_id, audience_raw, user_ids)

        await callback.message.edit_text(
            get("broadcast.starting", lang, audience=audience_display_name(audience, lang), recipients=total),
            reply_markup=_menu_keyboard(lang),
        )
    except Exception as e:
        logger.error("failed to enqueue broadcast task", broadcast_id=broadcast_id, error=e)
        await update_broadcast_status(broadcast_id, BroadcastStatus.FAILED)
        await callback.message.edit_text(
            get("broadcast.starting", lang, audience=audience_display_name(audience, lang), recipients=0),
            reply_markup=_menu_keyboard(lang),
        )

    await callback.answer()


# ---------------------------------------------------------------------------
# History
# ---------------------------------------------------------------------------


@broadcast_router.callback_query(F.data == "broadcast|history", _is_admin)
async def broadcast_history(callback: CallbackQuery, lang: str = "en"):
    broadcasts = await get_broadcasts_history(limit=20)
    if not broadcasts:
        await callback.message.edit_text(
            get("broadcast.history_empty", lang),
            reply_markup=InlineKeyboardMarkup(inline_keyboard=[[
                InlineKeyboardButton(
                    text=get("broadcast.btn_back", lang),
                    callback_data="broadcast|menu",
                )
            ]]),
        )
        await callback.answer()
        return

    lines = [get("broadcast.history_title", lang)]
    for b in broadcasts:
        created = b.created_at.strftime("%Y-%m-%d %H:%M") if b.created_at else "?"
        lines.append(
            get(
                "broadcast.history_line",
                lang,
                id=b.id,
                status=status_display_name(b.status, lang),
                audience=audience_display_name(b.audience, lang),
                sent=b.sent_count,
                total=b.total_recipients,
                created=created,
            )
        )
        if b.status == BroadcastStatus.COMPLETED and b.finished_at:
            duration = (b.finished_at - b.created_at).total_seconds() / 60
            lines.append(f"   {duration:.0f} min, {b.failed_count} failed, {b.blocked_count} blocked")

    await callback.message.edit_text(
        "\n".join(lines),
        reply_markup=InlineKeyboardMarkup(inline_keyboard=[[
            InlineKeyboardButton(
                text=get("broadcast.btn_back", lang),
                callback_data="broadcast|menu",
            )
        ]]),
    )
    await callback.answer()


# ---------------------------------------------------------------------------
# Active status
# ---------------------------------------------------------------------------


@broadcast_router.callback_query(F.data == "broadcast|status", _is_admin)
async def broadcast_active_status(callback: CallbackQuery, lang: str = "en"):
    active = await get_active_broadcast()
    if active is None:
        await callback.message.edit_text(
            get("broadcast.no_active", lang),
            reply_markup=InlineKeyboardMarkup(inline_keyboard=[[
                InlineKeyboardButton(
                    text=get("broadcast.btn_back", lang),
                    callback_data="broadcast|menu",
                )
            ]]),
        )
        await callback.answer()
        return

    stats = await get_broadcast_stats(active.id)
    await callback.message.edit_text(
        get(
            "broadcast.active_title",
            lang,
            id=active.id,
            status=status_display_name(active.status, lang),
            audience=audience_display_name(active.audience, lang),
            sent=active.sent_count or stats["sent"],
            total=active.total_recipients,
            failed=active.failed_count or stats["failed"],
            blocked=active.blocked_count or stats["blocked"],
        ),
        reply_markup=InlineKeyboardMarkup(inline_keyboard=[[
            InlineKeyboardButton(
                text=get("broadcast.btn_back", lang),
                callback_data="broadcast|menu",
            )
        ]]),
    )
    await callback.answer()

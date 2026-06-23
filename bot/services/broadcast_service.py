"""Database operations for the broadcast system."""

from datetime import datetime, UTC
from typing import Any

import structlog
from sqlalchemy import select, func, delete

from db.models import (
    Broadcast,
    BroadcastAudience,
    BroadcastDelivery,
    BroadcastDeliveryStatus,
    BroadcastStatus,
    User,
    utc_now_naive,
)
from db.session import async_session_factory
from bot.services.admin_log import log_broadcast_action

logger = structlog.get_logger()


def _now() -> datetime:
    return utc_now_naive()


# ---------------------------------------------------------------------------
# CRUD
# ---------------------------------------------------------------------------


async def create_broadcast(
    admin_id: int,
    text: str,
    audience: BroadcastAudience = BroadcastAudience.ALL,
) -> Broadcast:
    async with async_session_factory() as session:
        broadcast = Broadcast(
            admin_id=admin_id,
            text=text,
            audience=audience,
            status=BroadcastStatus.DRAFT,
            created_at=_now(),
        )
        session.add(broadcast)
        await session.commit()
        await session.refresh(broadcast)
        log_broadcast_action(admin_id, broadcast.id, "created", audience=audience.value)
        return broadcast


async def get_broadcast(broadcast_id: int) -> Broadcast | None:
    async with async_session_factory() as session:
        result = await session.execute(
            select(Broadcast).where(Broadcast.id == broadcast_id)
        )
        return result.scalar_one_or_none()


async def update_broadcast_status(
    broadcast_id: int,
    status: BroadcastStatus,
    **extra: Any,
) -> None:
    async with async_session_factory() as session:
        broadcast = await session.get(Broadcast, broadcast_id)
        if broadcast is None:
            return
        broadcast.status = status
        for key, value in extra.items():
            setattr(broadcast, key, value)
        await session.commit()


async def update_broadcast_text(broadcast_id: int, text: str) -> None:
    async with async_session_factory() as session:
        broadcast = await session.get(Broadcast, broadcast_id)
        if broadcast is None:
            return
        broadcast.text = text
        await session.commit()


async def get_broadcasts_history(limit: int = 20) -> list[Broadcast]:
    async with async_session_factory() as session:
        result = await session.execute(
            select(Broadcast)
            .order_by(Broadcast.created_at.desc())
            .limit(limit)
        )
        return list(result.scalars().all())


async def get_active_broadcast() -> Broadcast | None:
    async with async_session_factory() as session:
        result = await session.execute(
            select(Broadcast).where(
                Broadcast.status.in_([BroadcastStatus.QUEUED, BroadcastStatus.RUNNING])
            )
            .order_by(Broadcast.created_at.desc())
            .limit(1)
        )
        return result.scalar_one_or_none()


# ---------------------------------------------------------------------------
# Recipient counting & selection
# ---------------------------------------------------------------------------


async def count_recipients(audience: BroadcastAudience) -> int:
    async with async_session_factory() as session:
        query = select(func.count(User.id))
        if audience == BroadcastAudience.RU:
            query = query.where(User.language == "ru")
        elif audience == BroadcastAudience.EN:
            query = query.where(User.language == "en")
        elif audience == BroadcastAudience.ACTIVE:
            query = query.where(User.created_at.isnot(None))  # all users are active by default
        result = await session.execute(query)
        return result.scalar_one()


async def get_recipient_ids(audience: BroadcastAudience) -> list[int]:
    """Return list of user IDs matching the audience filter."""
    async with async_session_factory() as session:
        query = select(User.id)
        if audience == BroadcastAudience.RU:
            query = query.where(User.language == "ru")
        elif audience == BroadcastAudience.EN:
            query = query.where(User.language == "en")
        elif audience == BroadcastAudience.ACTIVE:
            query = query.where(User.created_at.isnot(None))
        result = await session.execute(query)
        return [row[0] for row in result.all()]


# ---------------------------------------------------------------------------
# Delivery tracking
# ---------------------------------------------------------------------------


async def create_delivery_records(broadcast_id: int, user_ids: list[int]) -> None:
    async with async_session_factory() as session:
        for uid in user_ids:
            session.add(BroadcastDelivery(
                broadcast_id=broadcast_id,
                user_id=uid,
                status=BroadcastDeliveryStatus.PENDING,
            ))
        await session.commit()


async def update_delivery(
    broadcast_id: int,
    user_id: int,
    status: BroadcastDeliveryStatus,
    error_message: str | None = None,
) -> None:
    async with async_session_factory() as session:
        result = await session.execute(
            select(BroadcastDelivery).where(
                BroadcastDelivery.broadcast_id == broadcast_id,
                BroadcastDelivery.user_id == user_id,
            ).limit(1)
        )
        delivery = result.scalar_one_or_none()
        if delivery is None:
            return
        delivery.status = status
        delivery.error_message = error_message
        if status == BroadcastDeliveryStatus.SENT:
            delivery.sent_at = _now()
        await session.commit()


async def get_broadcast_stats(broadcast_id: int) -> dict[str, int]:
    """Get current delivery stats for a broadcast."""
    async with async_session_factory() as session:
        deliveries = await session.execute(
            select(BroadcastDelivery.status).where(
                BroadcastDelivery.broadcast_id == broadcast_id
            )
        )
        rows = deliveries.all()
        sent = sum(1 for r in rows if r[0] == BroadcastDeliveryStatus.SENT)
        failed = sum(1 for r in rows if r[0] == BroadcastDeliveryStatus.FAILED)
        blocked = sum(1 for r in rows if r[0] == BroadcastDeliveryStatus.BLOCKED)
        return {
            "total": len(rows),
            "sent": sent,
            "failed": failed,
            "blocked": blocked,
        }


def audience_display_name(audience: BroadcastAudience, lang: str = "en") -> str:
    """Human-readable audience name for display."""
    from bot.locale import get as _get
    key = f"audience_label_{audience.value}"
    return _get(f"broadcast.{key}", lang)


def status_display_name(status: BroadcastStatus, lang: str = "en") -> str:
    """Human-readable status name for display."""
    from bot.locale import get as _get
    key = f"status_{status.value}"
    return _get(f"broadcast.{key}", lang)

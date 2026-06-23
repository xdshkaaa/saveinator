"""Celery task for executing info broadcasts to users.

Sends messages in batches with configurable delay between each to
respect Telegram rate limits.  Tracks delivery status per user and
updates broadcast-level counters in PostgreSQL.
"""

import asyncio
import time
from datetime import datetime, UTC

import structlog
from aiogram.exceptions import (
    TelegramForbiddenError,
    TelegramRetryAfter,
    TelegramAPIError,
)
from sqlalchemy import select

from bot.config import settings
from bot.locale import get
from bot.services.runtime_settings import get_runtime_value_sync
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
from workers.app import app
from workers.bot import get_bot
from workers.metrics import (
    BROADCASTS_TOTAL_WORKER,
    BROADCAST_MESSAGES_SENT_TOTAL_WORKER,
    BROADCAST_MESSAGES_FAILED_TOTAL_WORKER,
    BROADCAST_MESSAGES_BLOCKED_TOTAL_WORKER,
)

logger = structlog.get_logger()


def _now() -> datetime:
    return utc_now_naive()


def _get_broadcast_delay_ms() -> int:
    """Read the broadcast delay from runtime settings, with fallback."""
    val = get_runtime_value_sync("global.broadcast_delay_ms")
    if isinstance(val, int) and val > 0:
        return val
    return 50


def _get_broadcast_batch_size() -> int:
    """Read the broadcast batch size from runtime settings, with fallback."""
    val = get_runtime_value_sync("global.broadcast_batch_size")
    if isinstance(val, int) and val > 0:
        return val
    return 20


async def _send_broadcast(
    broadcast_id: int,
    audience_raw: str,
    user_ids: list[int],
) -> None:
    """Core async logic for sending a broadcast.

    Updates delivery records in DB and sends messages via the Bot API.
    """
    bot = get_bot()
    delay_ms = _get_broadcast_delay_ms()
    delay_sec = delay_ms / 1000.0
    audience = BroadcastAudience(audience_raw)

    # Read broadcast text
    async with async_session_factory() as session:
        broadcast = await session.get(Broadcast, broadcast_id)
        if broadcast is None:
            logger.error("broadcast not found", broadcast_id=broadcast_id)
            return
        text = broadcast.text

    # Mark as running
    async with async_session_factory() as session:
        broadcast = await session.get(Broadcast, broadcast_id)
        if broadcast is None:
            return
        broadcast.status = BroadcastStatus.RUNNING
        broadcast.started_at = _now()
        broadcast.total_recipients = len(user_ids)
        await session.commit()

    BROADCASTS_TOTAL_WORKER.labels(audience=audience_raw).inc()
    sent_count = 0
    failed_count = 0
    blocked_count = 0
    total = len(user_ids)

    for idx, user_id in enumerate(user_ids):
        try:
            await bot.send_message(chat_id=user_id, text=text)
            sent_count += 1
            BROADCAST_MESSAGES_SENT_TOTAL_WORKER.inc()

            # Update delivery record
            async with async_session_factory() as session:
                result = await session.execute(
                    select(BroadcastDelivery).where(
                        BroadcastDelivery.broadcast_id == broadcast_id,
                        BroadcastDelivery.user_id == user_id,
                    ).limit(1)
                )
                delivery = result.scalar_one_or_none()
                if delivery:
                    delivery.status = BroadcastDeliveryStatus.SENT
                    delivery.sent_at = _now()
                else:
                    session.add(BroadcastDelivery(
                        broadcast_id=broadcast_id,
                        user_id=user_id,
                        status=BroadcastDeliveryStatus.SENT,
                        sent_at=_now(),
                    ))
                await session.commit()

        except TelegramForbiddenError:
            # Bot was blocked by this user
            blocked_count += 1
            BROADCAST_MESSAGES_BLOCKED_TOTAL_WORKER.inc()
            async with async_session_factory() as session:
                result = await session.execute(
                    select(BroadcastDelivery).where(
                        BroadcastDelivery.broadcast_id == broadcast_id,
                        BroadcastDelivery.user_id == user_id,
                    ).limit(1)
                )
                delivery = result.scalar_one_or_none()
                if delivery:
                    delivery.status = BroadcastDeliveryStatus.BLOCKED
                else:
                    session.add(BroadcastDelivery(
                        broadcast_id=broadcast_id,
                        user_id=user_id,
                        status=BroadcastDeliveryStatus.BLOCKED,
                    ))
                await session.commit()

        except TelegramRetryAfter as e:
            # Telegram rate limit — wait and retry
            retry_after = e.retry_after
            logger.warning("broadcast rate limited", user_id=user_id, retry_after=retry_after)
            await asyncio.sleep(min(retry_after, 30))
            try:
                await bot.send_message(chat_id=user_id, text=text)
                sent_count += 1
                BROADCAST_MESSAGES_SENT_TOTAL_WORKER.inc()
            except Exception:
                failed_count += 1
                BROADCAST_MESSAGES_FAILED_TOTAL_WORKER.inc()
                async with async_session_factory() as session:
                    result = await session.execute(
                        select(BroadcastDelivery).where(
                            BroadcastDelivery.broadcast_id == broadcast_id,
                            BroadcastDelivery.user_id == user_id,
                        ).limit(1)
                    )
                    delivery = result.scalar_one_or_none()
                    if delivery:
                        delivery.status = BroadcastDeliveryStatus.FAILED
                        delivery.error_message = str(e)[:500]
                    await session.commit()

        except TelegramAPIError as e:
            failed_count += 1
            BROADCAST_MESSAGES_FAILED_TOTAL_WORKER.inc()
            async with async_session_factory() as session:
                result = await session.execute(
                    select(BroadcastDelivery).where(
                        BroadcastDelivery.broadcast_id == broadcast_id,
                        BroadcastDelivery.user_id == user_id,
                    ).limit(1)
                )
                delivery = result.scalar_one_or_none()
                if delivery:
                    delivery.status = BroadcastDeliveryStatus.FAILED
                    delivery.error_message = str(e)[:500]
                await session.commit()

        except Exception as e:
            failed_count += 1
            BROADCAST_MESSAGES_FAILED_TOTAL_WORKER.inc()
            logger.warning("broadcast send failed", user_id=user_id, error=str(e))

        # Rate limiting delay between messages
        if delay_sec > 0 and idx < total - 1:
            await asyncio.sleep(delay_sec)

        # Batch progress update every _get_broadcast_batch_size() users
        batch_size = _get_broadcast_batch_size()
        if batch_size > 0 and (idx + 1) % batch_size == 0:
            async with async_session_factory() as session:
                broadcast = await session.get(Broadcast, broadcast_id)
                if broadcast:
                    broadcast.sent_count = sent_count
                    broadcast.failed_count = failed_count
                    broadcast.blocked_count = blocked_count
                    await session.commit()
            logger.info(
                "broadcast progress",
                broadcast_id=broadcast_id,
                sent=sent_count,
                total=total,
                failed=failed_count,
                blocked=blocked_count,
            )

    # Mark as completed
    async with async_session_factory() as session:
        broadcast = await session.get(Broadcast, broadcast_id)
        if broadcast:
            broadcast.status = BroadcastStatus.COMPLETED
            broadcast.sent_count = sent_count
            broadcast.failed_count = failed_count
            broadcast.blocked_count = blocked_count
            broadcast.finished_at = _now()
            await session.commit()

    duration = (_now() - broadcast.created_at).total_seconds() / 60 if broadcast else 0
    logger.info(
        "broadcast completed",
        broadcast_id=broadcast_id,
        sent=sent_count,
        total=total,
        failed=failed_count,
        blocked=blocked_count,
        duration_min=round(duration, 1),
    )


@app.task(bind=True, max_retries=1, default_retry_delay=30)
def execute_broadcast(
    self,
    broadcast_id: int,
    audience: str,
    user_ids: list[int],
) -> None:
    """Celery task: execute a broadcast to a list of user IDs."""
    # Mark as running and create delivery records
    try:
        asyncio.run(_send_broadcast(broadcast_id, audience, user_ids))
    except Exception as e:
        logger.exception("broadcast task crashed", broadcast_id=broadcast_id, error=str(e))
        try:
            # Mark as failed
            import asyncio
            async def _mark_failed():
                from db.models import async_session_factory
                async with async_session_factory() as session:
                    broadcast = await session.get(Broadcast, broadcast_id)
                    if broadcast:
                        broadcast.status = BroadcastStatus.FAILED
                        broadcast.finished_at = _now()
                        await session.commit()
            asyncio.run(_mark_failed())
        except Exception:
            logger.exception("failed to mark broadcast as failed", broadcast_id=broadcast_id)

        raise

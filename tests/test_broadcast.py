"""Tests for the broadcast system — DB service, audience selection, handler."""

import pytest
from unittest.mock import AsyncMock, patch

from db.models import (
    User,
    BroadcastDelivery,
    BroadcastDeliveryStatus,
    BroadcastStatus,
    BroadcastAudience,
    utc_now_naive,
)
from bot.services.broadcast_service import (
    create_broadcast,
    get_broadcast,
    update_broadcast_status,
    count_recipients,
    get_recipient_ids,
    get_broadcasts_history,
    get_active_broadcast,
    get_broadcast_stats,
    audience_display_name,
    status_display_name,
)
from bot.filters.admin import IsAdminFilter


class FakeCallbackForFilter:
    def __init__(self, data: str):
        self.data = data


def _matching_broadcast_callbacks(data: str) -> list[str]:
    from bot.handlers.broadcast import broadcast_router

    callback = FakeCallbackForFilter(data)
    matches: list[str] = []
    for handler in broadcast_router.callback_query.handlers:
        magic_filter = next((flt.magic for flt in handler.filters if flt.magic is not None), None)
        if magic_filter is not None and magic_filter.resolve(callback):
            matches.append(handler.callback.__name__)
    return matches


# ---------------------------------------------------------------------------
# Helpers
# ---------------------------------------------------------------------------


@pytest.fixture
async def sample_users(db_session):
    """Create a few sample users with different languages."""
    users = [
        User(id=100, username="user_en", first_name="Alice", language="en"),
        User(id=200, username="user_ru", first_name="Боб", language="ru"),
        User(id=300, username="user_en2", first_name="Charlie", language="en"),
        User(id=400, username="user_ru2", first_name="Диана", language="ru"),
    ]
    for u in users:
        db_session.add(u)
    await db_session.commit()
    return users


# ---------------------------------------------------------------------------
# Broadcast CRUD
# ---------------------------------------------------------------------------


class TestCreateBroadcast:
    async def test_create_draft(self):
        broadcast = await create_broadcast(admin_id=999, text="Test message")
        assert broadcast.id is not None
        assert broadcast.admin_id == 999
        assert broadcast.text == "Test message"
        assert broadcast.status == BroadcastStatus.DRAFT
        assert broadcast.created_at is not None

    async def test_create_with_audience(self):
        broadcast = await create_broadcast(
            admin_id=999,
            text="RU only",
            audience=BroadcastAudience.RU,
        )
        assert broadcast.audience == BroadcastAudience.RU


class TestGetBroadcast:
    async def test_get_existing(self):
        created = await create_broadcast(admin_id=1, text="hello")
        fetched = await get_broadcast(created.id)
        assert fetched is not None
        assert fetched.id == created.id
        assert fetched.text == "hello"

    async def test_get_missing(self):
        fetched = await get_broadcast(99999)
        assert fetched is None


class TestUpdateStatus:
    async def test_update_to_running(self):
        b = await create_broadcast(admin_id=1, text="test")
        await update_broadcast_status(b.id, BroadcastStatus.RUNNING, started_at=utc_now_naive())
        fetched = await get_broadcast(b.id)
        assert fetched is not None
        assert fetched.status == BroadcastStatus.RUNNING
        assert fetched.started_at is not None


# ---------------------------------------------------------------------------
# Audience counting & selection
# ---------------------------------------------------------------------------


class TestCountRecipients:
    async def test_count_all(self, sample_users):
        count = await count_recipients(BroadcastAudience.ALL)
        assert count == 4

    async def test_count_en(self, sample_users):
        count = await count_recipients(BroadcastAudience.EN)
        assert count == 2

    async def test_count_ru(self, sample_users):
        count = await count_recipients(BroadcastAudience.RU)
        assert count == 2


class TestGetRecipientIds:
    async def test_all_users(self, sample_users):
        ids = await get_recipient_ids(BroadcastAudience.ALL)
        assert sorted(ids) == [100, 200, 300, 400]

    async def test_en_only(self, sample_users):
        ids = await get_recipient_ids(BroadcastAudience.EN)
        assert sorted(ids) == [100, 300]

    async def test_ru_only(self, sample_users):
        ids = await get_recipient_ids(BroadcastAudience.RU)
        assert sorted(ids) == [200, 400]


# ---------------------------------------------------------------------------
# History
# ---------------------------------------------------------------------------


class TestGetHistory:
    async def test_empty_history(self):
        history = await get_broadcasts_history(limit=10)
        assert history == []

    async def test_ordered_history(self):
        b1 = await create_broadcast(1, "first")
        b2 = await create_broadcast(2, "second")
        history = await get_broadcasts_history(limit=10)
        assert len(history) == 2
        # Most recent first
        assert history[0].id >= history[1].id


class TestGetActive:
    async def test_no_active(self):
        active = await get_active_broadcast()
        assert active is None

    async def test_active_exists(self):
        b = await create_broadcast(1, "active test")
        await update_broadcast_status(b.id, BroadcastStatus.RUNNING)
        active = await get_active_broadcast()
        assert active is not None
        assert active.id == b.id


class TestBroadcastStats:
    async def test_counts_delivery_statuses(self, db_session, sample_users):
        broadcast = await create_broadcast(1, "stats")
        db_session.add_all(
            [
                BroadcastDelivery(
                    broadcast_id=broadcast.id,
                    user_id=100,
                    status=BroadcastDeliveryStatus.SENT,
                ),
                BroadcastDelivery(
                    broadcast_id=broadcast.id,
                    user_id=200,
                    status=BroadcastDeliveryStatus.FAILED,
                ),
                BroadcastDelivery(
                    broadcast_id=broadcast.id,
                    user_id=300,
                    status=BroadcastDeliveryStatus.BLOCKED,
                ),
                BroadcastDelivery(
                    broadcast_id=broadcast.id,
                    user_id=400,
                    status=BroadcastDeliveryStatus.PENDING,
                ),
            ]
        )
        await db_session.commit()

        stats = await get_broadcast_stats(broadcast.id)

        assert stats == {
            "total": 4,
            "sent": 1,
            "failed": 1,
            "blocked": 1,
        }


# ---------------------------------------------------------------------------
# Display names
# ---------------------------------------------------------------------------


class TestDisplayNames:
    def test_audience_display_en(self):
        assert "All" in audience_display_name(BroadcastAudience.ALL, "en")

    def test_audience_display_ru(self):
        assert "Все" in audience_display_name(BroadcastAudience.ALL, "ru")

    def test_status_display_en(self):
        assert "Completed" in status_display_name(BroadcastStatus.COMPLETED, "en")

    def test_status_display_ru(self):
        assert "Завершена" in status_display_name(BroadcastStatus.COMPLETED, "ru")


class TestBroadcastRouting:
    def test_admin_menu_callback_opens_broadcast_menu(self):
        assert _matching_broadcast_callbacks("admin|broadcasts") == ["broadcast_menu"]

    def test_back_callback_returns_to_broadcast_menu(self):
        assert _matching_broadcast_callbacks("broadcast|menu") == ["broadcast_menu"]

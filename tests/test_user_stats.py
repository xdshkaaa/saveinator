"""Tests for admin user statistics service."""

from datetime import timedelta

import pytest

from db.models import (
    Chat,
    Download,
    DownloadStatus,
    Language,
    Platform,
    User,
    utc_now_naive,
)
from bot.services.user_stats import fetch_user_stats


@pytest.fixture
async def stats_seed(db_session, fake_redis):
    now = utc_now_naive()
    today = now.replace(hour=12, minute=0, second=0, microsecond=0)
    yesterday = today - timedelta(days=1)
    week_ago = now - timedelta(days=3)
    old = now - timedelta(days=40)

    users = [
        User(id=1, username="u1", first_name="A", language=Language.EN, created_at=today),
        User(id=2, username="u2", first_name="B", language=Language.RU, created_at=today),
        User(id=3, username="u3", first_name="C", language=Language.EN, created_at=yesterday),
        User(id=4, username="u4", first_name="D", language=Language.RU, created_at=week_ago),
        User(id=5, username="u5", first_name="E", language=Language.EN, created_at=old),
    ]
    chat = Chat(id=100, title="test", type="private", created_at=now)
    db_session.add(chat)
    for user in users:
        db_session.add(user)

    downloads = [
        Download(
            user_id=1,
            chat_id=100,
            url="https://youtube.com/1",
            platform=Platform.YOUTUBE,
            status=DownloadStatus.COMPLETED,
            created_at=now - timedelta(hours=2),
        ),
        Download(
            user_id=1,
            chat_id=100,
            url="https://youtube.com/2",
            platform=Platform.YOUTUBE,
            status=DownloadStatus.COMPLETED,
            created_at=now - timedelta(days=2),
        ),
        Download(
            user_id=2,
            chat_id=100,
            url="https://tiktok.com/1",
            platform=Platform.TIKTOK,
            status=DownloadStatus.COMPLETED,
            created_at=now - timedelta(hours=1),
        ),
        Download(
            user_id=3,
            chat_id=100,
            url="https://youtube.com/3",
            platform=Platform.YOUTUBE,
            status=DownloadStatus.COMPLETED,
            created_at=now - timedelta(days=10),
        ),
        Download(
            user_id=5,
            chat_id=100,
            url="https://youtube.com/old",
            platform=Platform.YOUTUBE,
            status=DownloadStatus.COMPLETED,
            created_at=old,
        ),
    ]
    for download in downloads:
        db_session.add(download)

    await db_session.commit()
    return users


class TestFetchUserStats:
    async def test_registered_and_growth(self, stats_seed):
        stats = await fetch_user_stats()

        assert stats.total_users == 5
        assert stats.new_today == 2
        assert stats.new_yesterday == 1
        assert stats.new_7d == 4
        assert stats.new_30d == 4

    async def test_activity_and_engagement(self, stats_seed):
        stats = await fetch_user_stats()

        assert stats.dau == 2
        assert stats.wau == 2
        assert stats.mau == 3
        assert stats.users_with_downloads == 4
        assert stats.returning_users == 1

    async def test_languages_and_platforms(self, stats_seed):
        stats = await fetch_user_stats()

        assert stats.language_en == 3
        assert stats.language_ru == 2
        assert stats.top_platforms_7d == [("youtube", 1), ("tiktok", 1)]

    async def test_banned_count(self, stats_seed, fake_redis):
        from bot.services.user_bans import ban_user

        await ban_user(99)
        stats = await fetch_user_stats()
        assert stats.banned_count == 1

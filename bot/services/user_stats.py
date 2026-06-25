"""Aggregate user statistics for the admin panel."""

from dataclasses import dataclass
from datetime import timedelta

from sqlalchemy import desc, func, select

from bot.metrics import get_active_user_count
from bot.services.user_bans import list_banned_users
from db.models import Download, DownloadStatus, Language, User, utc_now_naive
from db.session import async_session_factory


@dataclass(frozen=True)
class UserStatsSnapshot:
    total_users: int
    new_today: int
    new_yesterday: int
    new_7d: int
    new_30d: int
    active_now: int
    dau: int
    wau: int
    mau: int
    users_with_downloads: int
    returning_users: int
    language_en: int
    language_ru: int
    top_platforms_7d: list[tuple[str, int]]
    banned_count: int


def _cutoffs() -> dict[str, object]:
    now = utc_now_naive()
    today_start = now.replace(hour=0, minute=0, second=0, microsecond=0)
    return {
        "today_start": today_start,
        "yesterday_start": today_start - timedelta(days=1),
        "day_ago": now - timedelta(days=1),
        "week_ago": now - timedelta(days=7),
        "month_ago": now - timedelta(days=30),
    }


def _language_value(language: Language | str) -> str:
    if isinstance(language, Language):
        return language.value
    return str(language)


async def fetch_user_stats() -> UserStatsSnapshot:
    cutoffs = _cutoffs()
    active_now = get_active_user_count()
    banned_count = len(await list_banned_users())

    async with async_session_factory() as session:
        total_users = (await session.execute(select(func.count(User.id)))).scalar_one()

        new_today = (
            await session.execute(
                select(func.count(User.id)).where(
                    User.created_at >= cutoffs["today_start"]
                )
            )
        ).scalar_one()

        new_yesterday = (
            await session.execute(
                select(func.count(User.id)).where(
                    User.created_at >= cutoffs["yesterday_start"],
                    User.created_at < cutoffs["today_start"],
                )
            )
        ).scalar_one()

        new_7d = (
            await session.execute(
                select(func.count(User.id)).where(
                    User.created_at >= cutoffs["week_ago"]
                )
            )
        ).scalar_one()

        new_30d = (
            await session.execute(
                select(func.count(User.id)).where(
                    User.created_at >= cutoffs["month_ago"]
                )
            )
        ).scalar_one()

        async def _distinct_downloaders_since(since) -> int:
            result = await session.execute(
                select(func.count(func.distinct(Download.user_id))).where(
                    Download.created_at >= since
                )
            )
            return result.scalar_one()

        dau = await _distinct_downloaders_since(cutoffs["day_ago"])
        wau = await _distinct_downloaders_since(cutoffs["week_ago"])
        mau = await _distinct_downloaders_since(cutoffs["month_ago"])

        users_with_downloads = (
            await session.execute(
                select(func.count(func.distinct(Download.user_id)))
            )
        ).scalar_one()

        returning_subq = (
            select(Download.user_id)
            .where(Download.status == DownloadStatus.COMPLETED)
            .group_by(Download.user_id)
            .having(func.count() >= 2)
            .subquery()
        )
        returning_users = (
            await session.execute(select(func.count()).select_from(returning_subq))
        ).scalar_one()

        lang_rows = (
            await session.execute(
                select(User.language, func.count()).group_by(User.language)
            )
        ).all()
        language_en = 0
        language_ru = 0
        for language, count in lang_rows:
            if _language_value(language) == "en":
                language_en = count
            elif _language_value(language) == "ru":
                language_ru = count

        platform_count = func.count(func.distinct(Download.user_id)).label("user_count")
        platform_rows = (
            await session.execute(
                select(Download.platform, platform_count)
                .where(Download.created_at >= cutoffs["week_ago"])
                .group_by(Download.platform)
                .order_by(desc(platform_count))
                .limit(3)
            )
        ).all()
        top_platforms_7d = [
            (platform.value if hasattr(platform, "value") else str(platform), count)
            for platform, count in platform_rows
        ]

    return UserStatsSnapshot(
        total_users=total_users,
        new_today=new_today,
        new_yesterday=new_yesterday,
        new_7d=new_7d,
        new_30d=new_30d,
        active_now=active_now,
        dau=dau,
        wau=wau,
        mau=mau,
        users_with_downloads=users_with_downloads,
        returning_users=returning_users,
        language_en=language_en,
        language_ru=language_ru,
        top_platforms_7d=top_platforms_7d,
        banned_count=banned_count,
    )

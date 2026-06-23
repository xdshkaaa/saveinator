from typing import Any

from db.models import Language, User, UserSettings
from db.session import async_session_factory
from bot.metrics import record_user_created


_DEFAULT_QUALITY = "ask"
_DEFAULT_RATIO = "ask"


async def ensure_user(
    user_id: int,
    username: str | None = None,
    first_name: str | None = None,
    language: str = "en",
) -> User:
    async with async_session_factory() as session:
        user = await session.get(User, user_id)
        if user is not None:
            return user
        user = User(
            id=user_id,
            username=username,
            first_name=first_name,
            language=Language(language),
        )
        session.add(user)
        await session.commit()
        record_user_created()
        return user


async def get_or_create_user_settings(user_id: int) -> UserSettings:
    async with async_session_factory() as session:
        user = await session.get(User, user_id)
        if user is None:
            user = User(id=user_id)
            session.add(user)
            await session.flush()
            record_user_created()
        settings = await session.get(UserSettings, user_id)
        if settings is not None:
            return settings
        settings = UserSettings(user_id=user_id)
        session.add(settings)
        await session.commit()
        return settings


async def _ensure_user(
    user_id: int,
    session: Any,
) -> None:
    user = await session.get(User, user_id)
    if user is None:
        user = User(id=user_id)
        session.add(user)
        await session.flush()
        record_user_created()


async def set_user_language(user_id: int, language: str) -> None:
    async with async_session_factory() as session:
        await _ensure_user(user_id, session)
        user = await session.get(User, user_id)
        if user is None:
            return
        lang = Language.RU if language == "ru" else Language.EN
        user.language = lang
        await session.commit()


async def set_youtube_quality(user_id: int, quality: str) -> None:
    async with async_session_factory() as session:
        await _ensure_user(user_id, session)
        settings = await session.get(UserSettings, user_id)
        if settings is None:
            settings = UserSettings(user_id=user_id)
            session.add(settings)
        settings.youtube_quality = quality
        await session.commit()


async def set_youtube_ratio(user_id: int, ratio: str) -> None:
    async with async_session_factory() as session:
        await _ensure_user(user_id, session)
        settings = await session.get(UserSettings, user_id)
        if settings is None:
            settings = UserSettings(user_id=user_id)
            session.add(settings)
        settings.youtube_ratio = ratio
        await session.commit()


async def reset_user_settings(user_id: int) -> None:
    async with async_session_factory() as session:
        await _ensure_user(user_id, session)
        user = await session.get(User, user_id)
        if user is not None:
            user.language = Language.EN
        settings = await session.get(UserSettings, user_id)
        if settings is not None:
            settings.youtube_quality = _DEFAULT_QUALITY
            settings.youtube_ratio = _DEFAULT_RATIO
        else:
            settings = UserSettings(user_id=user_id)
            session.add(settings)
        await session.commit()

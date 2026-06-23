from db.models import User, UserSettings, Language
from db.session import async_session_factory
from bot.services.user_settings import (
    get_or_create_user_settings,
    set_user_language,
    set_youtube_quality,
    set_youtube_ratio,
    reset_user_settings,
    ensure_user,
)


async def test_default_settings_created(db_session):
    settings = await get_or_create_user_settings(999)
    assert settings.user_id == 999
    assert settings.youtube_quality == "ask"
    assert settings.youtube_ratio == "ask"

    user = await db_session.get(User, 999)
    assert user is not None


async def test_get_or_create_user_settings_twice(db_session):
    s1 = await get_or_create_user_settings(888)
    s2 = await get_or_create_user_settings(888)
    assert s1.user_id == s2.user_id
    assert s1.youtube_quality == s2.youtube_quality


async def test_set_youtube_quality(db_session):
    await get_or_create_user_settings(111)
    await set_youtube_quality(111, "1080")
    settings = await db_session.get(UserSettings, 111)
    assert settings is not None
    assert settings.youtube_quality == "1080"


async def test_set_youtube_quality_without_preexisting_user(db_session):
    await set_youtube_quality(112, "720")
    settings = await db_session.get(UserSettings, 112)
    assert settings is not None
    assert settings.youtube_quality == "720"

    user = await db_session.get(User, 112)
    assert user is not None


async def test_set_youtube_ratio(db_session):
    await get_or_create_user_settings(222)
    await set_youtube_ratio(222, "16_9")
    settings = await db_session.get(UserSettings, 222)
    assert settings is not None
    assert settings.youtube_ratio == "16_9"


async def test_set_user_language(db_session):
    await ensure_user(333, language="en")
    await set_user_language(333, "ru")
    user = await db_session.get(User, 333)
    assert user is not None
    assert user.language == Language.RU


async def test_set_user_language_back_to_en(db_session):
    await ensure_user(444, language="ru")
    await set_user_language(444, "en")
    user = await db_session.get(User, 444)
    assert user is not None
    assert user.language == Language.EN


async def test_reset_user_settings(db_session):
    await get_or_create_user_settings(555)
    await set_youtube_quality(555, "1080")
    await set_youtube_ratio(555, "16_9")
    await reset_user_settings(555)
    settings = await db_session.get(UserSettings, 555)
    assert settings is not None
    assert settings.youtube_quality == "ask"
    assert settings.youtube_ratio == "ask"


async def test_reset_user_settings_resets_language(db_session):
    await ensure_user(666, language="ru")
    await reset_user_settings(666)
    user = await db_session.get(User, 666)
    assert user is not None
    assert user.language == Language.EN


async def test_reset_user_settings_no_preexisting(db_session):
    await reset_user_settings(777)
    settings = await db_session.get(UserSettings, 777)
    assert settings is not None
    assert settings.youtube_quality == "ask"
    assert settings.youtube_ratio == "ask"


async def test_ensure_user_new(db_session):
    user = await ensure_user(888, username="testuser", first_name="Test", language="ru")
    assert user.id == 888
    assert user.username == "testuser"
    assert user.language == Language.RU


async def test_ensure_user_existing(db_session):
    await ensure_user(999, username="original")
    user2 = await ensure_user(999, username="updated")
    assert user2.username == "original"

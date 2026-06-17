from bot.services.youtube_session import (
    YoutubePendingSession,
    clear_youtube_session,
    get_youtube_session,
    save_youtube_session,
    update_youtube_quality,
)


async def test_save_and_get_youtube_session(fake_redis):
    session = YoutubePendingSession(
        user_id=10,
        url="https://youtu.be/abc12345678",
        chat_id=20,
        message_id=30,
        lang="ru",
    )
    await save_youtube_session(session)

    loaded = await get_youtube_session(10)
    assert loaded is not None
    assert loaded.url == session.url
    assert loaded.chat_id == 20
    assert loaded.message_id == 30
    assert loaded.quality is None


async def test_update_youtube_quality(fake_redis):
    session = YoutubePendingSession(
        user_id=11,
        url="https://www.youtube.com/watch?v=abc12345678",
        chat_id=21,
        message_id=31,
        lang="en",
    )
    await save_youtube_session(session)

    updated = await update_youtube_quality(11, 720)
    assert updated is not None
    assert updated.quality == 720

    loaded = await get_youtube_session(11)
    assert loaded is not None
    assert loaded.quality == 720


async def test_clear_youtube_session(fake_redis):
    session = YoutubePendingSession(
        user_id=12,
        url="https://www.youtube.com/watch?v=abc12345678",
        chat_id=22,
        message_id=32,
        lang="en",
    )
    await save_youtube_session(session)
    await clear_youtube_session(12)

    assert await get_youtube_session(12) is None

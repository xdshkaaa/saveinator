from unittest.mock import AsyncMock, MagicMock

import pytest
from aiogram.types import Message

from bot.middleware.user_ban import UserBanMiddleware


def _message(user_id: int, text: str = "hello") -> Message:
    message = MagicMock(spec=Message)
    message.from_user = MagicMock()
    message.from_user.id = user_id
    message.chat = MagicMock()
    message.chat.id = 1
    message.chat.type = "private"
    message.text = text
    message.answers: list[str] = []

    async def answer(text: str, **_kwargs):
        message.answers.append(text)
        return message

    message.answer = answer
    return message


@pytest.fixture
async def fake_redis(monkeypatch):
    import fakeredis.aioredis

    server = fakeredis.FakeServer()
    async_client = fakeredis.aioredis.FakeRedis(server=server, decode_responses=True)

    import fakeredis

    sync_client = fakeredis.FakeRedis(server=server, decode_responses=True)

    async def _async_redis():
        return async_client

    monkeypatch.setattr("bot.services.redis_client._async_redis", async_client)
    monkeypatch.setattr("bot.services.redis_client.get_async_redis", _async_redis)
    monkeypatch.setattr("bot.services.redis_client._sync_redis", sync_client)
    monkeypatch.setattr("bot.services.redis_client.get_sync_redis", lambda: sync_client)
    return async_client


async def test_banned_user_gets_shadow_reply(fake_redis, monkeypatch):
    from bot.config import Settings
    from bot.services.user_bans import ban_user

    monkeypatch.setattr(
        "bot.middleware.user_ban.settings",
        Settings(bot_token="test-token", admin_telegram_id=1),
    )
    await ban_user(42)

    message = _message(42)
    middleware = UserBanMiddleware()
    handler = AsyncMock(return_value="handled")
    notify_admin = AsyncMock()
    monkeypatch.setattr("bot.middleware.user_ban.notify_admin_banned_message", notify_admin)
    bot = AsyncMock()

    result = await middleware(handler, message, {"lang": "ru", "bot": bot})

    assert result is None
    handler.assert_not_called()
    notify_admin.assert_awaited_once_with(bot, message, lang="ru")
    assert message.answers == [
        '⚠️ Собеседник не видит ваше сообщение! Для отправки сообщения напишите: "Анчоус."'
    ]


async def test_non_banned_user_passes_through(fake_redis, monkeypatch):
    from bot.config import Settings

    monkeypatch.setattr(
        "bot.middleware.user_ban.settings",
        Settings(bot_token="test-token", admin_telegram_id=1),
    )

    message = _message(42)
    middleware = UserBanMiddleware()
    handler = AsyncMock(return_value="handled")

    result = await middleware(handler, message, {"lang": "ru"})

    assert result == "handled"
    handler.assert_called_once()
    assert message.answers == []

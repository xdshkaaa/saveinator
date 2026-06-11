from unittest.mock import AsyncMock, MagicMock

import pytest
from aiogram.types import Message

from bot.services.ban_notifications import notify_admin_banned_message


def _message(user_id: int, text: str = "secret link https://youtu.be/x") -> Message:
    message = MagicMock(spec=Message)
    message.message_id = 99
    message.text = text
    message.from_user = MagicMock()
    message.from_user.id = user_id
    message.from_user.username = "banned_user"
    message.from_user.full_name = "Banned User"
    message.chat = MagicMock()
    message.chat.id = 555
    message.chat.type = "private"
    message.chat.title = None
    return message


@pytest.fixture
def admin_settings(monkeypatch):
    from bot.config import Settings

    monkeypatch.setattr(
        "bot.services.ban_notifications.settings",
        Settings(bot_token="test-token", admin_telegram_id=9001),
    )


async def test_notify_admin_forwards_message(admin_settings):
    bot = AsyncMock()
    message = _message(42)

    await notify_admin_banned_message(bot, message, lang="ru")

    bot.send_message.assert_awaited_once()
    assert bot.send_message.await_args.args[0] == 9001
    assert "42" in bot.send_message.await_args.args[1]
    bot.forward_message.assert_awaited_once_with(
        chat_id=9001,
        from_chat_id=555,
        message_id=99,
    )

import pytest
from unittest.mock import AsyncMock

from aiogram.fsm.context import FSMContext
from aiogram.fsm.storage.memory import MemoryStorage
from aiogram.types import InlineKeyboardMarkup

from bot.handlers.admin import AdminEdit, cmd_admin
from bot.config import Settings


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


class FakeUser:
    def __init__(self, user_id: int):
        self.id = user_id
        self.username = "admin"
        self.first_name = "Admin"


class FakeChat:
    id = 1
    type = "private"


class FakeMessage:
    def __init__(self, user_id: int, text: str = "/admin"):
        self.from_user = FakeUser(user_id)
        self.chat = FakeChat()
        self.text = text
        self.answers: list[str] = []

    async def answer(self, text: str, reply_markup=None):
        self.answers.append(text)
        return self


@pytest.fixture
def admin_settings(monkeypatch):
    monkeypatch.setattr(
        "bot.config.settings",
        Settings(bot_token="test-token", admin_telegram_id=339193247),
    )


async def test_admin_command_available_for_admin_id(admin_settings):
    message = FakeMessage(339193247)
    storage = MemoryStorage()
    state = FSMContext(storage=storage, key="chat:1:user:339193247")

    await cmd_admin(message, state, lang="ru")

    assert message.answers
    assert "Runtime" in message.answers[0] or "runtime" in message.answers[0].lower()


async def test_admin_edit_saves_runtime_value(admin_settings, fake_redis, monkeypatch):
    from bot.handlers.admin import admin_edit_value

    message = FakeMessage(339193247, text="120")
    storage = MemoryStorage()
    state = FSMContext(storage=storage, key="chat:1:user:339193247")
    await state.set_state(AdminEdit.waiting_value)
    await state.update_data(redis_key="youtube.timeout_sec", service="youtube")

    monkeypatch.setattr(
        "bot.handlers.admin._service_keyboard",
        lambda *_args, **_kwargs: InlineKeyboardMarkup(inline_keyboard=[]),
    )
    monkeypatch.setattr(
        "bot.handlers.admin._service_summary",
        AsyncMock(return_value="summary"),
    )

    await admin_edit_value(message, state, lang="en")

    from bot.services.runtime_settings import platform_download_timeout_seconds

    assert platform_download_timeout_seconds("youtube") == 120

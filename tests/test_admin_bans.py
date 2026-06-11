import pytest
from unittest.mock import AsyncMock

from aiogram.fsm.context import FSMContext
from aiogram.fsm.storage.memory import MemoryStorage
from aiogram.types import InlineKeyboardMarkup

from bot.config import Settings
from bot.handlers.admin import AdminBan, admin_ban_add_value, admin_bans_menu
from bot.services.user_bans import ban_user, list_banned_users


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

    async def reply(self, text: str):
        self.answers.append(text)
        return self


class FakeCallbackMessage:
    def __init__(self):
        self.text = ""
        self.edits: list[str] = []

    async def edit_text(self, text: str, reply_markup=None):
        self.text = text
        self.edits.append(text)
        return self


class FakeCallback:
    def __init__(self, data: str):
        self.data = data
        self.message = FakeCallbackMessage()
        self.answers: list[str] = []

    async def answer(self, text: str | None = None):
        if text:
            self.answers.append(text)


@pytest.fixture
def admin_settings(monkeypatch):
    monkeypatch.setattr(
        "bot.config.settings",
        Settings(bot_token="test-token", admin_telegram_id=339193247),
    )


async def test_admin_ban_add_value(admin_settings, fake_redis, monkeypatch):
    message = FakeMessage(339193247, text="555")
    storage = MemoryStorage()
    state = FSMContext(storage=storage, key="chat:1:user:339193247")
    await state.set_state(AdminBan.waiting_ban_id)

    monkeypatch.setattr(
        "bot.handlers.admin._bans_keyboard",
        lambda *_args, **_kwargs: InlineKeyboardMarkup(inline_keyboard=[]),
    )
    monkeypatch.setattr(
        "bot.handlers.admin._bans_summary",
        AsyncMock(return_value="summary"),
    )

    await admin_ban_add_value(message, state, lang="ru")

    assert await list_banned_users() == [555]
    assert any("555" in answer for answer in message.answers)


async def test_admin_bans_menu_lists_banned_users(admin_settings, fake_redis):
    await ban_user(777)
    callback = FakeCallback("admin|bans")

    await admin_bans_menu(callback, state=FSMContext(
        storage=MemoryStorage(),
        key="chat:1:user:339193247",
    ), lang="ru")

    assert "777" in callback.message.text

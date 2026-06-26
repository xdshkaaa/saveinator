import pytest
from unittest.mock import AsyncMock

from aiogram.fsm.context import FSMContext
from aiogram.fsm.storage.memory import MemoryStorage
from aiogram.types import InlineKeyboardMarkup

from bot.handlers.admin import AdminEdit, _main_keyboard, admin_stats, cmd_admin, cmd_stats
from bot.config import Settings
from bot.services.runtime_settings import SERVICE_ORDER


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


async def test_main_keyboard_includes_users_button(admin_settings):
    keyboard = _main_keyboard("en")
    labels = [button.text for row in keyboard.inline_keyboard for button in row]
    assert any("Users" in label for label in labels)


async def test_main_keyboard_two_column_layout(admin_settings):
    keyboard = _main_keyboard("en")
    rows = keyboard.inline_keyboard

    assert len(rows) == 6
    assert all(1 <= len(row) <= 2 for row in rows)
    assert all(len(row) == 2 for row in rows[:-1])

    callbacks = [button.callback_data for row in rows for button in row]
    expected = [
        *[f"admin|svc|{service}" for service in SERVICE_ORDER],
        "admin|svc|global",
        "admin|broadcasts",
        "admin|stats",
        "admin|bans",
        "admin|confirm|reset_all",
    ]
    assert callbacks == expected


class FakeCallbackMessage:
    def __init__(self):
        self.text: str | None = None
        self.reply_markup = None

    async def edit_text(self, text: str, reply_markup=None):
        self.text = text
        self.reply_markup = reply_markup
        return self


class FakeCallback:
    def __init__(self, user_id: int, data: str):
        self.from_user = FakeUser(user_id)
        self.data = data
        self.message = FakeCallbackMessage()
        self.answers: list[str] = []

    async def answer(self, text: str | None = None):
        if text is not None:
            self.answers.append(text)


async def test_admin_stats_callback_renders_snapshot(admin_settings, monkeypatch):
    from bot.services.user_stats import UserStatsSnapshot

    snapshot = UserStatsSnapshot(
        total_users=10,
        new_today=2,
        new_yesterday=1,
        new_7d=5,
        new_30d=8,
        active_now=3,
        dau=4,
        wau=6,
        mau=7,
        users_with_downloads=9,
        returning_users=2,
        language_en=6,
        language_ru=4,
        top_platforms_7d=[("youtube", 3)],
        banned_count=0,
    )
    monkeypatch.setattr(
        "bot.handlers.admin.fetch_user_stats",
        AsyncMock(return_value=snapshot),
    )

    callback = FakeCallback(339193247, "admin|stats")
    storage = MemoryStorage()
    state = FSMContext(storage=storage, key="chat:1:user:339193247")

    await admin_stats(callback, state, lang="en")

    assert callback.message.text is not None
    assert "Registered: 10" in callback.message.text
    assert callback.message.reply_markup is not None


async def test_stats_command_available_for_admin(admin_settings, monkeypatch):
    from bot.services.user_stats import UserStatsSnapshot

    snapshot = UserStatsSnapshot(
        total_users=1,
        new_today=0,
        new_yesterday=0,
        new_7d=1,
        new_30d=1,
        active_now=0,
        dau=0,
        wau=0,
        mau=0,
        users_with_downloads=0,
        returning_users=0,
        language_en=1,
        language_ru=0,
        top_platforms_7d=[],
        banned_count=0,
    )
    monkeypatch.setattr(
        "bot.handlers.admin.fetch_user_stats",
        AsyncMock(return_value=snapshot),
    )

    message = FakeMessage(339193247, text="/stats")
    storage = MemoryStorage()
    state = FSMContext(storage=storage, key="chat:1:user:339193247")

    await cmd_stats(message, state, lang="en")

    assert message.answers
    assert "User statistics" in message.answers[0]

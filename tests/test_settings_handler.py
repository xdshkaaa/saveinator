from bot.handlers.settings import cmd_settings
from bot.handlers.settings import (
    settings_menu,
    settings_lang_menu,
    settings_lang_choose,
    settings_quality_menu,
    settings_quality_choose,
    settings_ratio_menu,
    settings_ratio_choose,
    settings_reset,
)
from bot.handlers.group import handle_group_message
from bot.services.user_settings import get_or_create_user_settings, set_youtube_quality, set_youtube_ratio


class FakeUser:
    id = 20


class FakeChat:
    id = 10


class FakeSentMessage:
    message_id = 999


class FakeBot:
    sent_messages: list[tuple] = []

    async def send_message(self, chat_id: int, text: str):
        FakeBot.sent_messages.append((chat_id, text))
        return FakeSentMessage()


class FakeStatusMessage:
    message_id = 30

    def __init__(self):
        self.text = ""
        self.reply_markup = None

    async def edit_text(self, text: str, reply_markup=None):
        self.text = text
        self.reply_markup = reply_markup


class FakeMessage:
    chat = FakeChat()
    from_user = FakeUser()
    bot = FakeBot()

    def __init__(self, text: str):
        self.text = text
        self.answers: list[str] = []
        self.replies: list[str] = []
        self.reply_markups: list[object] = []

    async def answer(self, text: str, reply_markup=None):
        self.answers.append(text)
        self.reply_markups.append(reply_markup)
        return self

    async def reply(self, text: str, reply_markup=None):
        self.replies.append(text)
        self.reply_markups.append(reply_markup)
        return FakeStatusMessage()


class FakeCallbackUser:
    id = 20


class FakeCallbackMessage(FakeStatusMessage):
    chat = FakeChat()


class FakeCallbackQuery:
    def __init__(self, data: str):
        self.data = data
        self.from_user = FakeCallbackUser()
        self.message = FakeCallbackMessage()
        self.bot = FakeBot()
        self.answers: list[tuple] = []

    async def answer(self, text: str | None = None, show_alert: bool = False):
        self.answers.append((text, show_alert))


async def test_settings_command_shows_menu(fake_redis):
    message = FakeMessage("/settings")
    await cmd_settings(message, None, lang="en")
    assert message.answers
    assert "Settings" in message.answers[0]


async def test_settings_command_russian(fake_redis):
    message = FakeMessage("/settings")
    await cmd_settings(message, None, lang="ru")
    assert message.answers
    assert "Настройки" in message.answers[0]


async def test_settings_menu_callback(fake_redis):
    callback = FakeCallbackQuery("settings|menu")
    await settings_menu(callback, None, lang="en")
    assert callback.message.text
    assert "Settings" in callback.message.text


async def test_settings_lang_menu_shows_languages(fake_redis):
    callback = FakeCallbackQuery("settings|lang")
    await settings_lang_menu(callback, lang="en")
    assert "Choose interface" in callback.message.text


async def test_settings_lang_choose_ru(fake_redis):
    callback = FakeCallbackQuery("settings|lang|ru")
    await settings_lang_choose(callback, lang="en")
    assert callback.message.text
    assert "Настройки" in callback.message.text or "язык" in callback.message.text


async def test_settings_lang_choose_en(fake_redis):
    callback = FakeCallbackQuery("settings|lang|en")
    await settings_lang_choose(callback, lang="ru")
    assert callback.message.text
    assert "Settings" in callback.message.text or "Language" in callback.message.text


async def test_settings_quality_menu(fake_redis):
    callback = FakeCallbackQuery("settings|quality")
    await settings_quality_menu(callback, lang="en")
    assert "quality" in callback.message.text.lower() or "YouTube" in callback.message.text


async def test_settings_quality_choose_1080(fake_redis, db_session):
    callback = FakeCallbackQuery("settings|quality|1080")
    await settings_quality_choose(callback, lang="en")
    assert callback.message.text
    assert "1080" in callback.message.text


async def test_settings_quality_choose_ask(fake_redis, db_session):
    callback = FakeCallbackQuery("settings|quality|ask")
    await settings_quality_choose(callback, lang="en")
    assert callback.message.text
    assert "Ask" in callback.message.text


async def test_settings_ratio_menu(fake_redis):
    callback = FakeCallbackQuery("settings|ratio")
    await settings_ratio_menu(callback, lang="en")
    assert "ratio" in callback.message.text.lower() or "format" in callback.message.text.lower()


async def test_settings_ratio_choose_16_9(fake_redis, db_session):
    callback = FakeCallbackQuery("settings|ratio|16_9")
    await settings_ratio_choose(callback, lang="en")
    assert callback.message.text
    assert "16:9" in callback.message.text


async def test_settings_reset(fake_redis, db_session):
    await set_youtube_quality(20, "1080")
    await set_youtube_ratio(20, "16_9")
    callback = FakeCallbackQuery("settings|reset")
    await settings_reset(callback, lang="en")
    settings = await get_or_create_user_settings(20)
    assert settings.youtube_quality == "ask"
    assert settings.youtube_ratio == "ask"


async def test_callbacks_use_localized_keys(fake_redis):
    callback = FakeCallbackQuery("settings|menu")
    await settings_menu(callback, None, lang="ru")
    assert callback.message.text
    assert "Настройки" in callback.message.text

    callback2 = FakeCallbackQuery("settings|menu")
    await settings_menu(callback2, None, lang="en")
    assert callback2.message.text
    assert "Settings" in callback2.message.text


async def test_youtube_flow_respects_saved_quality_ratio(monkeypatch, fake_redis, db_session):
    """When both quality and ratio are saved, YouTube download starts directly."""
    await set_youtube_quality(20, "1080")
    await set_youtube_ratio(20, "16_9")

    delayed: list[dict] = []
    FakeBot.sent_messages = []
    message = FakeMessage("https://www.youtube.com/watch?v=dQw4w9WgXcQ")

    monkeypatch.setattr(
        "bot.handlers.group.download_and_send_task.delay",
        lambda **kwargs: delayed.append(kwargs),
    )

    await handle_group_message(message, lang="en")

    # Should auto-download without showing quality/ratio menus
    assert delayed
    assert delayed[0]["quality"] == 1080
    assert delayed[0]["aspect_ratio"] == "16_9"
    # Processing message was replied, no quality/ratio menus shown
    assert "16:9" in message.replies[0]


async def test_youtube_flow_show_ratio_menu_when_quality_saved(monkeypatch, fake_redis, db_session):
    """When only quality is saved, skip quality menu and show ratio menu."""
    from bot.services.youtube_session import get_youtube_session

    await set_youtube_quality(20, "720")

    delayed: list[dict] = []
    FakeBot.sent_messages = []
    message = FakeMessage("https://www.youtube.com/watch?v=dQw4w9WgXcQ")

    monkeypatch.setattr(
        "bot.handlers.group.download_and_send_task.delay",
        lambda **kwargs: delayed.append(kwargs),
    )

    await handle_group_message(message, lang="ru")

    # Should show ratio menu directly, no quality menu
    assert delayed == []
    assert message.replies
    assert "соотношение" in message.replies[0]

    # Session should have quality pre-set
    session = await get_youtube_session(20)
    assert session is not None
    assert session.quality == 720


async def test_youtube_flow_ask_both_default(monkeypatch, fake_redis, db_session):
    """Default behavior: show quality menu when nothing is saved."""
    delayed: list[dict] = []
    message = FakeMessage("https://www.youtube.com/watch?v=dQw4w9WgXcQ")

    monkeypatch.setattr(
        "bot.handlers.group.download_and_send_task.delay",
        lambda **kwargs: delayed.append(kwargs),
    )

    await handle_group_message(message, lang="en")

    assert delayed == []
    assert message.replies == ["Choose video quality:"]
    assert message.reply_markups[0] is not None

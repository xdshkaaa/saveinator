from bot.handlers.group import handle_group_message
from bot.handlers.youtube import handle_quality_choice, handle_ratio_choice


class FakeUser:
    id = 20


class FakeChat:
    id = 10


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

    def __init__(self, text: str):
        self.text = text
        self.replies: list[str] = []
        self.reply_markups: list[object] = []

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
        self.answers: list[tuple] = []

    async def answer(self, text: str | None = None, show_alert: bool = False):
        self.answers.append((text, show_alert))


async def _acquire_lock(*_args, **_kwargs):
    return "test-lock-token"


async def _not_busy(_user_id):
    return False


async def test_youtube_link_shows_quality_menu(monkeypatch, fake_redis):
    delayed: list[dict] = []
    message = FakeMessage("https://www.youtube.com/watch?v=dQw4w9WgXcQ")

    monkeypatch.setattr("bot.handlers.group.is_user_busy", _not_busy)
    monkeypatch.setattr(
        "bot.handlers.group.download_and_send_task.delay",
        lambda **kwargs: delayed.append(kwargs),
    )

    await handle_group_message(message, lang="ru")

    assert delayed == []
    assert message.replies == ["Выберите качество видео:"]
    assert message.reply_markups[0] is not None


async def test_tiktok_link_still_starts_download_task(monkeypatch, fake_redis):
    delayed: list[dict] = []
    message = FakeMessage("https://vt.tiktok.com/ZSxv29fme/")

    monkeypatch.setattr("bot.handlers.group.acquire_user_lock", _acquire_lock)
    monkeypatch.setattr(
        "bot.handlers.group.download_and_send_task.delay",
        lambda **kwargs: delayed.append(kwargs),
    )

    await handle_group_message(message, lang="en")

    assert delayed
    assert delayed[0]["platform"] == "tiktok"


async def test_quality_callback_updates_ratio_menu(monkeypatch, fake_redis):
    from bot.services.youtube_session import YoutubePendingSession, save_youtube_session

    await save_youtube_session(
        YoutubePendingSession(
            user_id=20,
            url="https://www.youtube.com/watch?v=dQw4w9WgXcQ",
            chat_id=10,
            message_id=30,
            lang="ru",
        )
    )

    callback = FakeCallbackQuery("quality:720")
    await handle_quality_choice(callback, lang="ru")

    assert callback.message.text == "Выберите соотношение сторон:"
    assert callback.message.reply_markup is not None


async def test_ratio_callback_starts_youtube_download(monkeypatch, fake_redis):
    from bot.services.youtube_session import YoutubePendingSession, save_youtube_session

    await save_youtube_session(
        YoutubePendingSession(
            user_id=20,
            url="https://www.youtube.com/watch?v=dQw4w9WgXcQ",
            chat_id=10,
            message_id=30,
            lang="ru",
            quality=720,
        )
    )

    delayed: list[dict] = []
    monkeypatch.setattr("bot.handlers.youtube.acquire_user_lock", _acquire_lock)
    monkeypatch.setattr(
        "bot.handlers.youtube.download_and_send_task.delay",
        lambda **kwargs: delayed.append(kwargs),
    )

    callback = FakeCallbackQuery("ratio:16_9")
    await handle_ratio_choice(callback, lang="ru")

    assert delayed
    assert delayed[0]["platform"] == "youtube"
    assert delayed[0]["quality"] == 720
    assert delayed[0]["aspect_ratio"] == "16_9"
    assert "720p" in callback.message.text
    assert "16:9" in callback.message.text

import asyncio

from bot.services.user_queue import UserScenario


class FakeUser:
    def __init__(self, user_id: int):
        self.id = user_id


class FakeCallbackMessage:
    def __init__(self):
        self.edited_texts: list[str] = []
        self.reply_markup = object()

    async def edit_text(self, text: str, reply_markup=None):
        self.edited_texts.append(text)
        self.reply_markup = reply_markup


class FakeCallback:
    def __init__(self, data: str, user_id: int = 20):
        self.data = data
        self.from_user = FakeUser(user_id)
        self.message = FakeCallbackMessage()
        self.answers: list[tuple[str | None, bool]] = []

    async def answer(self, text: str | None = None, show_alert: bool | None = None):
        self.answers.append((text, bool(show_alert)))


async def test_cancel_callback_cancels_active_download_and_clears_status(monkeypatch):
    from bot.handlers.download_cancel import cancel_download
    from bot.services.download_cancel import build_cancel_callback_data, register_download_task

    released: list[tuple[int, str, UserScenario]] = []

    async def _release(user_id: int, token: str, scenario: UserScenario):
        released.append((user_id, token, scenario))

    monkeypatch.setattr("bot.handlers.download_cancel.release_user_lock", _release)

    async def _sleep_forever():
        await asyncio.Event().wait()

    task = asyncio.create_task(_sleep_forever())
    token = "tok123"
    data = build_cancel_callback_data(UserScenario.SOUNDCLOUD, 20, token)
    register_download_task(UserScenario.SOUNDCLOUD, 20, token, task)

    callback = FakeCallback(data)
    await cancel_download(callback, lang="en")
    await asyncio.sleep(0)

    assert task.cancelled()
    assert released == [(20, token, UserScenario.SOUNDCLOUD)]
    assert callback.message.edited_texts == ["Download cancelled."]
    assert callback.message.reply_markup is None
    assert callback.answers[-1] == ("Cancelled", False)


async def test_cancel_callback_rejects_other_user(monkeypatch):
    from bot.handlers.download_cancel import cancel_download
    from bot.services.download_cancel import build_cancel_callback_data, register_download_task

    async def _never_release(*_args, **_kwargs):
        raise AssertionError("should not release another user's lock")

    monkeypatch.setattr("bot.handlers.download_cancel.release_user_lock", _never_release)

    async def _sleep_forever():
        await asyncio.Event().wait()

    task = asyncio.create_task(_sleep_forever())
    token = "tok123"
    data = build_cancel_callback_data(UserScenario.SPOTIFY, 20, token)
    register_download_task(UserScenario.SPOTIFY, 20, token, task)

    callback = FakeCallback(data, user_id=99)
    await cancel_download(callback, lang="en")

    assert not task.cancelled()
    assert callback.message.edited_texts == []
    assert callback.answers[-1] == ("This download belongs to another user.", True)
    task.cancel()


async def test_user_queue_returns_active_download(fake_redis):
    from bot.services.user_queue import acquire_user_lock, get_active_user_download

    token = await acquire_user_lock(20, UserScenario.SOUNDCLOUD)

    active = await get_active_user_download(20)

    assert active is not None
    assert active.user_id == 20
    assert active.scenario == UserScenario.SOUNDCLOUD
    assert active.token == token


async def test_busy_reply_has_download_queue_button(monkeypatch):
    from bot.handlers.group import _acquire_scenario_lock

    class FakeBusyMessage:
        from_user = FakeUser(20)
        replies: list[tuple[str, object | None]] = []

        async def reply(self, text: str, reply_markup=None):
            self.replies.append((text, reply_markup))

    async def _locked(*_args, **_kwargs):
        return None

    monkeypatch.setattr("bot.handlers.group.acquire_user_lock", _locked)

    message = FakeBusyMessage()
    token = await _acquire_scenario_lock(message, "en", UserScenario.SPOTIFY)

    assert token is None
    text, markup = message.replies[-1]
    assert text == "Please wait until your current download finishes before sending another link."
    assert markup is not None
    button = markup.inline_keyboard[0][0]
    assert button.text == "My downloads"
    assert button.callback_data == "dlq:20"


async def test_queue_callback_shows_active_download_button(fake_redis):
    from bot.handlers.download_cancel import show_download_queue
    from bot.services.download_cancel import build_queue_callback_data
    from bot.services.user_queue import acquire_user_lock

    token = await acquire_user_lock(20, UserScenario.SOUNDCLOUD)
    callback = FakeCallback(build_queue_callback_data(20), user_id=20)

    await show_download_queue(callback, lang="en")

    assert callback.message.edited_texts == ["Your active downloads:"]
    button = callback.message.reply_markup.inline_keyboard[0][0]
    assert button.text == "Remove SoundCloud download"
    assert button.callback_data == f"dlc:soundcloud:20:{token}"
    assert callback.answers[-1] == (None, False)


async def test_queue_callback_rejects_other_user():
    from bot.handlers.download_cancel import show_download_queue
    from bot.services.download_cancel import build_queue_callback_data

    callback = FakeCallback(build_queue_callback_data(20), user_id=99)

    await show_download_queue(callback, lang="en")

    assert callback.message.edited_texts == []
    assert callback.answers[-1] == ("This queue belongs to another user.", True)

from bot.handlers.group import handle_group_message


class FakeUser:
    id = 20


class FakeChat:
    id = 10


class FakeStatusMessage:
    message_id = 30


class FakeMessage:
    chat = FakeChat()
    from_user = FakeUser()

    def __init__(self, text: str = "https://www.instagram.com/reel/DXyPEDrMKV1/?igsh=MTlmNzlwc2lqeGhtMA=="):
        self.text = text
        self.replies: list[str] = []

    async def reply(self, text: str):
        self.replies.append(text)
        return FakeStatusMessage()


async def _acquire_lock(*_args, **_kwargs):
    return "test-lock-token"


async def test_instagram_reel_starts_download_task(monkeypatch):
    delayed: list[dict] = []
    message = FakeMessage()

    monkeypatch.setattr("bot.handlers.group.acquire_user_lock", _acquire_lock)
    monkeypatch.setattr(
        "bot.handlers.group.download_and_send_task.delay",
        lambda **kwargs: delayed.append(kwargs),
    )

    await handle_group_message(message, lang="en")

    assert delayed
    assert delayed[0]["platform"] == "instagram"
    assert delayed[0]["url"] == message.text
    assert not any("Unsupported link" in reply for reply in message.replies)


async def test_x_status_starts_download_task(monkeypatch):
    delayed: list[dict] = []
    message = FakeMessage("https://x.com/user/status/1234567890123456789")

    monkeypatch.setattr("bot.handlers.group.acquire_user_lock", _acquire_lock)
    monkeypatch.setattr(
        "bot.handlers.group.download_and_send_task.delay",
        lambda **kwargs: delayed.append(kwargs),
    )

    await handle_group_message(message, lang="en")

    assert delayed
    assert delayed[0]["platform"] == "x"
    assert delayed[0]["url"] == message.text
    assert delayed[0]["x_status_id"] == "1234567890123456789"
    assert not any("Unsupported link" in reply for reply in message.replies)


async def test_x_reply_link_passes_x_status_id(monkeypatch):
    delayed: list[dict] = []
    message = FakeMessage("https://twitter.com/user/status/9876543210987654321?s=20")

    monkeypatch.setattr("bot.handlers.group.acquire_user_lock", _acquire_lock)
    monkeypatch.setattr(
        "bot.handlers.group.download_and_send_task.delay",
        lambda **kwargs: delayed.append(kwargs),
    )

    await handle_group_message(message, lang="en")

    assert delayed
    assert delayed[0]["platform"] == "x"
    assert delayed[0]["x_status_id"] == "9876543210987654321"

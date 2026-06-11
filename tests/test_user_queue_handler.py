from bot.handlers.group import handle_group_message


class FakeUser:
    id = 77


class FakeChat:
    id = 55


class FakeMessage:
    chat = FakeChat()
    from_user = FakeUser()

    def __init__(self, text: str):
        self.text = text
        self.replies: list[str] = []

    async def reply(self, text: str, reply_markup=None):
        self.replies.append(text)
        return self


async def test_busy_user_gets_wait_message_for_video(monkeypatch):
    message = FakeMessage("https://vt.tiktok.com/ZSxv29fme/")
    delayed: list[dict] = []

    async def _busy_lock(*_args, **_kwargs):
        return None

    monkeypatch.setattr("bot.handlers.group.acquire_user_lock", _busy_lock)
    monkeypatch.setattr(
        "bot.handlers.group.download_and_send_task.delay",
        lambda **kwargs: delayed.append(kwargs),
    )

    await handle_group_message(message, lang="en")

    assert delayed == []
    assert message.replies
    assert "current download" in message.replies[0].lower()

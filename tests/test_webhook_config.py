from types import SimpleNamespace

from bot.main import _webhook_secret_token, _webhook_url, on_startup


class FakeBot:
    def __init__(self):
        self.deleted_webhook_kwargs = None
        self.webhook_kwargs = None
        self.commands = None
        self.menu_button = None

    async def delete_webhook(self, **kwargs):
        self.deleted_webhook_kwargs = kwargs

    async def set_webhook(self, url, **kwargs):
        self.webhook_kwargs = {"url": url, **kwargs}

    async def set_my_commands(self, commands, **kwargs):
        self.commands = list(commands)

    async def set_chat_menu_button(self, **kwargs):
        self.menu_button = kwargs.get("menu_button")


def test_webhook_url_normalizes_host_and_path(monkeypatch):
    monkeypatch.setattr(
        "bot.main.settings",
        SimpleNamespace(
            webhook_host="https://saveinator-hooks.xdshka.party/",
            webhook_path="webhook",
            webhook_secret_token="secret",
        ),
    )

    assert _webhook_url() == "https://saveinator-hooks.xdshka.party/webhook"
    assert _webhook_secret_token() == "secret"


def test_empty_webhook_secret_is_disabled(monkeypatch):
    monkeypatch.setattr(
        "bot.main.settings",
        SimpleNamespace(
            webhook_host="https://saveinator-hooks.xdshka.party",
            webhook_path="/webhook",
            webhook_secret_token="",
        ),
    )

    assert _webhook_secret_token() is None


async def test_startup_sets_production_webhook_with_secret(monkeypatch):
    monkeypatch.setattr(
        "bot.main.settings",
        SimpleNamespace(
            use_polling=False,
            webhook_host="https://saveinator-hooks.xdshka.party",
            webhook_path="/webhook",
            webhook_secret_token="secret-token",
        ),
    )
    bot = FakeBot()

    await on_startup(bot)

    assert bot.deleted_webhook_kwargs == {"drop_pending_updates": True}
    assert bot.webhook_kwargs == {
        "url": "https://saveinator-hooks.xdshka.party/webhook",
        "drop_pending_updates": True,
        "secret_token": "secret-token",
    }
    assert bot.commands
    assert bot.menu_button is not None

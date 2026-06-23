import asyncio
import logging
import sys

from aiohttp import web
from aiogram import Bot
from aiogram.webhook.aiohttp_server import SimpleRequestHandler, setup_application
from aiogram.types import BotCommand, BotCommandScopeDefault, MenuButtonCommands

from bot.api import register_download_routes
from bot.config import settings
from bot.dispatcher import create_dispatcher
from bot.metrics_server import start_metrics_server
from bot.telegram_instrumentation import instrument_bot

logger = logging.getLogger(__name__)


def _webhook_path() -> str:
    if settings.webhook_path.startswith("/"):
        return settings.webhook_path
    return f"/{settings.webhook_path}"


def _webhook_url() -> str:
    return f"{settings.webhook_host.rstrip('/')}{_webhook_path()}"


def _webhook_secret_token() -> str | None:
    return settings.webhook_secret_token or None


async def _register_bot_commands(bot: Bot) -> None:
    await bot.set_my_commands(
        [
            BotCommand(command="start", description="Start / language"),
            BotCommand(command="settings", description="User settings"),
        ],
        scope=BotCommandScopeDefault(),
    )


async def on_startup(bot: Bot):
    await bot.delete_webhook(drop_pending_updates=True)
    if not settings.use_polling:
        await bot.set_webhook(
            _webhook_url(),
            drop_pending_updates=True,
            secret_token=_webhook_secret_token(),
        )
    await _register_bot_commands(bot)
    await bot.set_chat_menu_button(menu_button=MenuButtonCommands())


async def on_shutdown(bot: Bot):
    if not settings.use_polling:
        await bot.delete_webhook()
    from bot.services.redis_client import close_async_redis
    await close_async_redis()


async def health(_request: web.Request) -> web.Response:
    return web.Response(text="ok")


async def run_webhook(dp, bot: Bot):
    if settings.metrics_enabled:
        await start_metrics_server()

    app = web.Application()
    app.router.add_get("/", health)
    app.router.add_get("/health", health)
    if settings.download_api_enabled:
        register_download_routes(app)
    webhook_requests_handler = SimpleRequestHandler(
        dispatcher=dp,
        bot=bot,
        secret_token=_webhook_secret_token(),
    )
    webhook_requests_handler.register(app, path=_webhook_path())
    setup_application(app, dp, bot=bot)
    dp.startup.register(on_startup)
    dp.shutdown.register(on_shutdown)
    runner = web.AppRunner(app)
    await runner.setup()
    site = web.TCPSite(runner, host=settings.webhook_listen, port=settings.webhook_port)
    await site.start()
    logger.info("Webhook server running on %s:%s", settings.webhook_listen, settings.webhook_port)
    await asyncio.Event().wait()


async def run_polling(dp, bot: Bot):
    await bot.delete_webhook(drop_pending_updates=True)
    await _register_bot_commands(bot)
    await bot.set_chat_menu_button(menu_button=MenuButtonCommands())
    if settings.metrics_enabled:
        await start_metrics_server()
    await dp.start_polling(bot)


async def main():
    logging.basicConfig(level=settings.log_level, stream=sys.stdout)
    bot = Bot(token=settings.bot_token)
    instrument_bot(bot)
    dp = create_dispatcher()

    if settings.use_polling:
        logger.info("Starting in polling mode")
        await run_polling(dp, bot)
    else:
        logger.info("Starting in webhook mode")
        await run_webhook(dp, bot)


if __name__ == "__main__":
    asyncio.run(main())

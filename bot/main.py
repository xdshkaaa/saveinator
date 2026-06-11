import asyncio
import logging
import sys

from aiohttp import web
from aiogram import Bot
from aiogram.webhook.aiohttp_server import SimpleRequestHandler, setup_application
from aiogram.types import MenuButtonCommands

from bot.config import settings
from bot.dispatcher import create_dispatcher
from bot.metrics_server import start_metrics_server
from bot.telegram_instrumentation import instrument_bot

logger = logging.getLogger(__name__)


async def on_startup(bot: Bot):
    await bot.delete_webhook(drop_pending_updates=True)
    if not settings.use_polling:
        await bot.set_webhook(
            f"{settings.webhook_host}{settings.webhook_path}",
            drop_pending_updates=True,
        )
    await bot.set_chat_menu_button(menu_button=MenuButtonCommands())


async def on_shutdown(bot: Bot):
    if not settings.use_polling:
        await bot.delete_webhook()


async def health(_request: web.Request) -> web.Response:
    return web.Response(text="ok")


async def run_webhook(dp, bot: Bot):
    from prometheus_client import CONTENT_TYPE_LATEST, generate_latest

    from bot.metrics import refresh_uptime

    app = web.Application()
    app.router.add_get("/health", health)

    async def metrics_handler(_request: web.Request) -> web.Response:
        refresh_uptime()
        return web.Response(
            body=generate_latest(),
            headers={"Content-Type": CONTENT_TYPE_LATEST},
        )

    app.router.add_get("/metrics", metrics_handler)
    webhook_requests_handler = SimpleRequestHandler(dispatcher=dp, bot=bot)
    webhook_requests_handler.register(app, path=settings.webhook_path)
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

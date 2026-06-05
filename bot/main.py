import asyncio
import logging
import sys

from aiohttp import web
from aiogram import Bot
from aiogram.webhook.aiohttp_server import SimpleRequestHandler, setup_application
from aiogram.types import MenuButtonCommands

from bot.config import settings
from bot.dispatcher import create_dispatcher

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
    app = web.Application()
    app.router.add_get("/health", health)
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
    await dp.start_polling(bot)


async def main():
    logging.basicConfig(level=settings.log_level, stream=sys.stdout)
    bot = Bot(token=settings.bot_token)
    dp = create_dispatcher()

    if settings.use_polling:
        logger.info("Starting in polling mode")
        await run_polling(dp, bot)
    else:
        logger.info("Starting in webhook mode")
        await run_webhook(dp, bot)


if __name__ == "__main__":
    asyncio.run(main())

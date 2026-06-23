import structlog
from aiogram import Bot

from bot.config import settings

logger = structlog.get_logger()

_bot: Bot | None = None


def get_bot() -> Bot:
    global _bot
    if _bot is None:
        _bot = Bot(token=settings.bot_token)
    return _bot


async def close_bot() -> None:
    global _bot
    if _bot is not None:
        try:
            await _bot.session.close()
        except Exception:
            logger.warning("error closing worker bot session", exc_info=True)
        finally:
            _bot = None
            logger.info("worker bot session closed")

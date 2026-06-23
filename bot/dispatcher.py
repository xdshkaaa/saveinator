from aiogram import Dispatcher

from bot.handlers.admin import admin_router
from bot.handlers.broadcast import broadcast_router
from bot.handlers.onboarding import onboarding_router
from bot.handlers.settings import settings_router
from bot.handlers.youtube import youtube_router
from bot.handlers.download_cancel import download_cancel_router
from bot.handlers.group import group_router
from bot.handlers.errors import errors_router
from bot.middleware.locale import LocaleMiddleware
from bot.middleware.metrics import MetricsMiddleware
from bot.middleware.rate_limit import RateLimitMiddleware
from bot.middleware.spam import SpamMiddleware
from bot.middleware.user_ban import UserBanMiddleware


def create_dispatcher() -> Dispatcher:
    dp = Dispatcher()

    dp.update.outer_middleware(MetricsMiddleware())

    dp.message.middleware(LocaleMiddleware())
    dp.callback_query.middleware(LocaleMiddleware())

    dp.message.middleware(RateLimitMiddleware())
    dp.message.outer_middleware(SpamMiddleware())
    dp.message.outer_middleware(UserBanMiddleware())

    dp.include_router(admin_router)
    dp.include_router(broadcast_router)
    dp.include_router(onboarding_router)
    dp.include_router(settings_router)
    dp.include_router(youtube_router)
    dp.include_router(download_cancel_router)
    dp.include_router(group_router)
    dp.include_router(errors_router)

    return dp

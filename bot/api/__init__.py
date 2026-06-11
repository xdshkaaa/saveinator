from aiohttp import web

from bot.api.pinterest import register_pinterest_routes


def register_download_routes(app: web.Application) -> None:
    register_pinterest_routes(app)

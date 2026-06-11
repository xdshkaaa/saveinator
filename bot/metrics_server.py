import asyncio
import logging

from aiohttp import web
from prometheus_client import CONTENT_TYPE_LATEST, generate_latest

from bot.api import register_download_routes
from bot.config import settings
from bot.metrics import init_platform_metrics, refresh_uptime

logger = logging.getLogger(__name__)


async def health(_request: web.Request) -> web.Response:
    return web.Response(text="ok")


async def metrics(_request: web.Request) -> web.Response:
    refresh_uptime()
    body = generate_latest()
    return web.Response(body=body, headers={"Content-Type": CONTENT_TYPE_LATEST})


async def start_metrics_server() -> web.AppRunner:
    init_platform_metrics()
    app = web.Application()
    app.router.add_get("/health", health)
    app.router.add_get("/metrics", metrics)
    if settings.download_api_enabled:
        register_download_routes(app)

    runner = web.AppRunner(app)
    await runner.setup()
    site = web.TCPSite(runner, host=settings.metrics_host, port=settings.metrics_port)
    await site.start()
    logger.info(
        "Metrics server listening on %s:%s",
        settings.metrics_host,
        settings.metrics_port,
    )
    return runner


async def run_metrics_server_background() -> None:
    if not settings.metrics_enabled:
        return
    await start_metrics_server()
    await asyncio.Event().wait()

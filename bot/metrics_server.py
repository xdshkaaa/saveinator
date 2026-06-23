import asyncio
import logging
import time

from aiohttp import web
from prometheus_client import CONTENT_TYPE_LATEST, generate_latest

from bot.api import register_download_routes
from bot.config import settings
from bot.metrics import (
    HTTP_REQUEST_LATENCY_SECONDS,
    HTTP_REQUESTS_TOTAL,
    init_platform_metrics,
    refresh_active_chats,
    refresh_uptime,
)

logger = logging.getLogger(__name__)


async def health(_request: web.Request) -> web.Response:
    return web.Response(text="ok")


async def metrics(_request: web.Request) -> web.Response:
    refresh_uptime()
    refresh_active_chats()
    body = generate_latest()
    return web.Response(body=body, headers={"Content-Type": CONTENT_TYPE_LATEST})


def _route_label(request: web.Request) -> str:
    resource = getattr(request.match_info.route, "resource", None)
    canonical = getattr(resource, "canonical", None)
    if canonical:
        return canonical
    return request.path


@web.middleware
async def metrics_middleware(
    request: web.Request,
    handler: web.RequestHandler,
) -> web.StreamResponse:
    if request.path == "/metrics":
        return await handler(request)

    started = time.perf_counter()
    status = "500"
    try:
        response = await handler(request)
        status = str(response.status)
        return response
    except web.HTTPException as exc:
        status = str(exc.status)
        raise
    finally:
        route = _route_label(request)
        HTTP_REQUESTS_TOTAL.labels(
            method=request.method,
            route=route,
            status=status,
        ).inc()
        HTTP_REQUEST_LATENCY_SECONDS.labels(
            method=request.method,
            route=route,
        ).observe(time.perf_counter() - started)


async def start_metrics_server() -> web.AppRunner:
    init_platform_metrics()
    app = web.Application(middlewares=[metrics_middleware])
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

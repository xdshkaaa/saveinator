import asyncio
import logging
import uuid
from pathlib import Path

from aiohttp import web
from pydantic import ValidationError

from bot.api.schemas import PinterestDownloadRequest
from bot.config import settings
from bot.services.pinterest_parser import is_valid_pinterest_url
from bot.services.tempfiles import tempfile_manager
from workers.pinterest_downloader import (
    PinterestDownloadError,
    PinterestNoMediaError,
    download_pinterest,
)

logger = logging.getLogger(__name__)


async def download_pinterest_handler(request: web.Request) -> web.Response:
    if not settings.pinterest_enabled:
        return web.json_response(
            {"error": "Pinterest downloads are disabled"},
            status=503,
        )

    try:
        payload = await request.json()
    except Exception:
        return web.json_response({"error": "Invalid JSON body"}, status=400)

    try:
        body = PinterestDownloadRequest.model_validate(payload)
    except ValidationError as exc:
        return web.json_response(
            {"error": "Invalid request", "details": exc.errors()},
            status=400,
        )

    if not is_valid_pinterest_url(body.url):
        return web.json_response(
            {"error": "Invalid or unsupported Pinterest URL"},
            status=400,
        )

    task_id = str(uuid.uuid4())

    def _run_download():
        with tempfile_manager(task_id, keep_on_success=True) as task_dir:
            return download_pinterest(
                body.url,
                Path(task_dir),
                max_items=body.limit,
                download_images=body.download_images,
                download_videos=body.download_videos,
            )

    try:
        result = await asyncio.wait_for(
            asyncio.to_thread(_run_download),
            timeout=settings.pinterest_timeout_seconds,
        )
    except TimeoutError:
        return web.json_response(
            {"error": f"Download exceeded {settings.pinterest_timeout_seconds}s timeout"},
            status=504,
        )
    except PinterestNoMediaError as exc:
        return web.json_response({"error": str(exc)}, status=404)
    except PinterestDownloadError as exc:
        message = str(exc)
        status = 403 if "private" in message.lower() else 422
        return web.json_response({"error": message}, status=status)
    except Exception:
        logger.exception("Pinterest API download failed")
        return web.json_response({"error": "Pinterest download failed"}, status=500)

    return web.json_response(result.to_dict())


def register_pinterest_routes(app: web.Application) -> None:
    app.router.add_post("/download/pinterest", download_pinterest_handler)

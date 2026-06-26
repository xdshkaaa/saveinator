import logging
import tempfile
from pathlib import Path

from bot.config import settings
from workers.app import app
from workers.downloader import build_ydl_opts, download

logger = logging.getLogger(__name__)


@app.task
def instagram_refresh_cookies_task():
    """Probe an Instagram URL to keep cookies/session warm."""
    if not settings.instagram_cookies_refresh_enabled:
        return
    cookie_path = settings.instagram_cookies_path.strip()
    browser = settings.instagram_cookies_from_browser.strip()
    if not cookie_path and not browser:
        return
    probe_url = settings.instagram_cookies_refresh_url.strip()
    if not probe_url:
        return
    try:
        with tempfile.TemporaryDirectory() as tmp:
            output_dir = Path(tmp)
            opts = build_ydl_opts(output_dir, format_id=None, platform="instagram")
            opts["skip_download"] = True
            import yt_dlp
            with yt_dlp.YoutubeDL(opts) as ydl:
                ydl.extract_info(probe_url, download=False)
    except Exception:
        logger.exception("instagram cookie refresh failed", extra={"url": probe_url})
        return
    logger.info("instagram cookie refresh ok", extra={"url": probe_url})

import logging
from pathlib import Path

import yt_dlp

logger = logging.getLogger(__name__)


def build_ydl_opts(output_dir: Path, format_id: str | None = None) -> dict:
    opts: dict = {
        "outtmpl": str(output_dir / "%(title).100s_%(id)s.%(ext)s"),
        "quiet": True,
        "no_warnings": True,
        "noplaylist": True,
        "extract_flat": False,
        "ignoreerrors": False,
    }
    if format_id:
        opts["format"] = format_id
        opts["merge_output_format"] = "mp4"
    return opts


import logging
from pathlib import Path

import yt_dlp

logger = logging.getLogger(__name__)


def build_ydl_opts(output_dir: Path, format_id: str | None = None) -> dict:
    opts: dict = {
        "outtmpl": str(output_dir / "%(title).100s_%(id)s.%(ext)s"),
        "quiet": True,
        "no_warnings": True,
        "noplaylist": True,
        "extract_flat": False,
        "ignoreerrors": False,
    }
    if format_id:
        opts["format"] = format_id
        opts["merge_output_format"] = "mp4"
    return opts


def download(url: str, output_dir: Path, format_id: str) -> dict:
    opts = build_ydl_opts(output_dir, format_id=format_id)
    try:
        with yt_dlp.YoutubeDL(opts) as ydl:
            return ydl.extract_info(url, download=True)
    except Exception:
        logger.exception("yt-dlp download failed", extra={"url": url, "format_id": format_id})
        raise

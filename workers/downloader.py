import json
import logging
from pathlib import Path

import yt_dlp

from bot.config import settings

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
    return opts


def fetch_info(url: str) -> dict:
    opts = build_ydl_opts(Path("/tmp"))
    with yt_dlp.YoutubeDL(opts) as ydl:
        return ydl.extract_info(url, download=False)


def download(url: str, output_dir: Path, format_id: str) -> dict:
    opts = build_ydl_opts(output_dir, format_id=format_id)
    with yt_dlp.YoutubeDL(opts) as ydl:
        return ydl.extract_info(url, download=True)


def check_playlist(url: str) -> bool:
    opts = {
        "quiet": True,
        "no_warnings": True,
        "extract_flat": True,
    }
    with yt_dlp.YoutubeDL(opts) as ydl:
        info = ydl.extract_info(url, download=False)
        return info.get("_type") == "playlist" or bool(info.get("entries"))

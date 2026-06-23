import logging
import re
from pathlib import Path
from urllib.parse import urlparse

import yt_dlp

logger = logging.getLogger(__name__)

_YTDLP_COMMON_OPTS: dict = {
    "quiet": True,
    "no_warnings": True,
    "noplaylist": True,
    "extract_flat": False,
    "ignoreerrors": False,
    "js_runtimes": {"deno": {}},
    "remote_components": {"ejs:github"},
    "postprocessor_args": {"ffmpeg": ["-threads", "1"]},
}

_X_STATUS_ID_REGEX = re.compile(r"/status/(\d+)")
_X_NATIVE_EXTRACTOR_KEYS = frozenset({"twitter", "x"})
_X_HOSTS = frozenset({"x.com", "twitter.com", "mobile.twitter.com"})


class XTargetReplyNotFoundError(Exception):
    """Target X/Twitter reply tweet not found in the thread."""


class XTargetReplyNoMediaError(Exception):
    """Target X/Twitter reply tweet contains no downloadable media."""


def build_ydl_opts(output_dir: Path, format_id: str | None = None) -> dict:
    opts: dict = {
        **_YTDLP_COMMON_OPTS,
        "outtmpl": str(output_dir / "%(title).100s_%(id)s.%(ext)s"),
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


def _extract_status_id_from_url(url: str) -> str | None:
    """Extract the numeric tweet/status ID from an X/Twitter URL."""
    if match := _X_STATUS_ID_REGEX.search(url):
        return match.group(1)
    return None


def _entry_matches_status_id(entry: dict, status_id: str) -> bool:
    """Check if a yt-dlp entry corresponds to the given tweet status ID."""
    if entry.get("id") == status_id:
        return True
    if entry.get("display_id") == status_id:
        return True
    for key in ("webpage_url", "original_url", "url"):
        url = entry.get(key) or ""
        if _extract_status_id_from_url(url) == status_id:
            return True
    return False


def _is_x_status_url(url: str, status_id: str) -> bool:
    parsed = urlparse(url)
    host = (parsed.hostname or "").lower()
    if host.startswith("www."):
        host = host[4:]
    return host in _X_HOSTS and _extract_status_id_from_url(url) == status_id


def _is_native_x_result(entry: dict, status_id: str) -> bool:
    """Return True when yt-dlp metadata points at native X/Twitter media."""
    extractor_key = str(entry.get("extractor_key") or "").lower()
    if extractor_key in _X_NATIVE_EXTRACTOR_KEYS:
        return True

    webpage_url = str(entry.get("webpage_url") or "")
    if webpage_url and _is_x_status_url(webpage_url, status_id):
        return True

    if extractor_key:
        return False

    return str(entry.get("id") or "") == status_id or str(entry.get("display_id") or "") == status_id


def _entry_has_media(entry: dict) -> bool:
    """Check if a yt-dlp entry contains downloadable media (not just text)."""
    return bool(entry.get("url") or entry.get("formats"))


def _find_entry_index(entries: list[dict], status_id: str) -> int | None:
    """Find the 1-based index of the entry matching the given status ID."""
    for idx, entry in enumerate(entries, 1):
        if _entry_matches_status_id(entry, status_id):
            return idx
    return None


def download_with_reply_filter(
    url: str,
    output_dir: Path,
    format_id: str,
    x_status_id: str,
) -> dict:
    """Download media from X/Twitter, selecting only the target reply.

    Handles the case where a reply URL expands to a full thread:
      1. Extract metadata (no download) with ``noplaylist=False`` to
         discover all entries.
      2. Find the entry whose ID matches *x_status_id*.
      3. Raise ``XTargetReplyNotFoundError`` if the entry is absent.
      4. Raise ``XTargetReplyNoMediaError`` if the entry has no
         downloadable media.
      5. Download only that single entry with ``playlist_items``.

    When the URL resolves to a single tweet (no *entries* key), the
    function falls through to a normal download.
    """
    meta_opts = build_ydl_opts(output_dir, format_id=format_id)
    meta_opts["noplaylist"] = False

    with yt_dlp.YoutubeDL(meta_opts) as ydl:
        info = ydl.extract_info(url, download=False)

    entries = info.get("entries")
    if not entries:
        if not _is_native_x_result(info, x_status_id):
            raise XTargetReplyNoMediaError(
                f"No native X/Twitter media in target tweet {x_status_id}"
            )
        # Single tweet — download as-is
        dl_opts = build_ydl_opts(output_dir, format_id=format_id)
        with yt_dlp.YoutubeDL(dl_opts) as ydl:
            return ydl.extract_info(url, download=True)

    # Thread — find the target entry
    target_idx = _find_entry_index(entries, x_status_id)
    if target_idx is None:
        raise XTargetReplyNotFoundError(
            f"Target X/Twitter reply {x_status_id} not found in thread"
        )

    target_entry = entries[target_idx - 1]
    if not _is_native_x_result(target_entry, x_status_id):
        raise XTargetReplyNoMediaError(
            f"No native X/Twitter media in target tweet {x_status_id}"
        )
    if not _entry_has_media(target_entry):
        raise XTargetReplyNoMediaError(
            f"No downloadable media in target X/Twitter reply {x_status_id}"
        )

    dl_opts = build_ydl_opts(output_dir, format_id=format_id)
    dl_opts["noplaylist"] = False
    dl_opts["playlist_items"] = str(target_idx)

    with yt_dlp.YoutubeDL(dl_opts) as ydl:
        return ydl.extract_info(url, download=True)

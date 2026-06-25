"""Download X/Twitter photo posts (yt-dlp handles video/GIF only)."""

import re
from pathlib import Path

import httpx
import structlog

from bot.services.runtime_settings import get_runtime_int

logger = structlog.get_logger()

_STATUS_ID_RE = re.compile(r"/status/(\d+)")
_IMAGE_EXTENSIONS = frozenset({".jpg", ".jpeg", ".png", ".webp"})
_API_TIMEOUT = 30.0
_IMAGE_TIMEOUT = 60

_FXTWITTER_API = "https://api.fxtwitter.com/status"
_VXTWITTER_API = "https://api.vxtwitter.com/status"
_API_HEADERS = {"User-Agent": "Mozilla/5.0 (compatible; Saveinator/1.0)"}


class XPhotosNotFoundError(Exception):
    """Tweet has no downloadable photos."""


class XPhotoDownloadError(Exception):
    """Failed to download photo files for the tweet."""


def extract_status_id(url: str) -> str | None:
    if match := _STATUS_ID_RE.search(url):
        return match.group(1)
    return None


def _parse_fxtwitter(data: dict) -> tuple[str, list[str]]:
    if data.get("code") not in (None, 200):
        raise ValueError(data.get("message") or "fxtwitter API error")

    tweet = data.get("tweet") or {}
    media = tweet.get("media") or {}
    photos = media.get("photos") or [
        item for item in media.get("all", []) if item.get("type") == "photo"
    ]
    urls = [photo["url"] for photo in photos if photo.get("url")]
    title = (tweet.get("text") or "").strip() or "x-post"
    return title, urls


def _parse_vxtwitter(data: dict) -> tuple[str, list[str]]:
    urls = list(data.get("mediaURLs") or [])
    for item in data.get("media_extended") or []:
        if item.get("type") == "image" and item.get("url") and item["url"] not in urls:
            urls.append(item["url"])
    title = (data.get("text") or "").strip() or "x-post"
    return title, urls


def fetch_x_photo_urls(status_id: str) -> tuple[str, list[str]]:
    """Return tweet title and original-quality photo URLs."""
    errors: list[str] = []
    for api_base, parser in (
        (_FXTWITTER_API, _parse_fxtwitter),
        (_VXTWITTER_API, _parse_vxtwitter),
    ):
        try:
            with httpx.Client(timeout=_API_TIMEOUT, headers=_API_HEADERS) as client:
                response = client.get(f"{api_base}/{status_id}")
                response.raise_for_status()
                title, urls = parser(response.json())
            if urls:
                return title, urls
            errors.append(f"{api_base}: empty photo list")
        except Exception as exc:
            logger.warning("x photo metadata fetch failed", api=api_base, error=str(exc))
            errors.append(f"{api_base}: {exc}")

    detail = f" ({'; '.join(errors)})" if errors else ""
    raise XPhotosNotFoundError(f"No photos found for tweet {status_id}{detail}")


def _guess_extension(url: str) -> str:
    path = url.split("?", 1)[0].lower()
    for ext in _IMAGE_EXTENSIONS:
        if path.endswith(ext):
            return ext
    return ".jpg"


def _download_image(url: str, output_path: Path) -> bool:
    try:
        with httpx.Client(
            timeout=_IMAGE_TIMEOUT,
            headers=_API_HEADERS,
            follow_redirects=True,
        ) as client:
            response = client.get(url)
            response.raise_for_status()
            output_path.write_bytes(response.content)
        return output_path.stat().st_size > 0
    except Exception as exc:
        logger.warning("failed to download x photo", url=url, error=str(exc))
        return False


def download_x_photos(
    url: str,
    output_dir: Path,
    *,
    status_id: str | None = None,
) -> dict:
    """Download photo attachments from an X/Twitter status URL."""
    sid = status_id or extract_status_id(url)
    if not sid:
        raise XPhotosNotFoundError("Cannot extract X status ID from URL")

    max_items = get_runtime_int("x.max_items_per_post")
    title, photo_urls = fetch_x_photo_urls(sid)
    if max_items > 0:
        photo_urls = photo_urls[:max_items]

    if not photo_urls:
        raise XPhotosNotFoundError(f"No photos in tweet {sid}")

    downloaded = 0
    for index, photo_url in enumerate(photo_urls, start=1):
        path = output_dir / f"photo_{index}{_guess_extension(photo_url)}"
        if _download_image(photo_url, path):
            downloaded += 1

    if downloaded == 0:
        raise XPhotoDownloadError(f"Failed to download photos for tweet {sid}")

    return {"title": title, "id": sid}

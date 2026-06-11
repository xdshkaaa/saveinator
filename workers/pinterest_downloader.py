import logging
from datetime import UTC, datetime
from pathlib import Path

from pinterest_dl import PinterestDL
from pinterest_dl.domain.media import PinterestMedia

from bot.config import settings
from bot.services.pinterest_models import (
    PinterestDownloadResult,
    PinterestMediaItem,
    PinterestUrlType,
)
from bot.services.pinterest_parser import parse_pinterest_url
from bot.services.pinterest_pin_fetcher import fetch_pin_media
from pinterest_dl.scrapers import operations

logger = logging.getLogger(__name__)

_VIDEO_EXTENSIONS = frozenset({".mp4", ".webm", ".mov", ".m4v"})
_IMAGE_EXTENSIONS = frozenset({".jpg", ".jpeg", ".png", ".webp", ".gif", ".bmp"})
_SINGLE_ITEM_URL_TYPES = frozenset({PinterestUrlType.PIN, PinterestUrlType.SHORT})


class PinterestDownloadError(Exception):
    """Raised when a Pinterest URL cannot be downloaded."""


class PinterestNoMediaError(PinterestDownloadError):
    """Raised when scraping succeeds but no downloadable media is found."""


def _media_type(item: PinterestMedia) -> str:
    if item.local_path and item.local_path.suffix.lower() in _IMAGE_EXTENSIONS:
        return "image"
    if item.local_path and item.local_path.suffix.lower() in _VIDEO_EXTENSIONS:
        return "video"
    if item.video_stream is not None:
        return "video"
    return "image"


def _pick_primary_item(items: list[PinterestMediaItem]) -> list[PinterestMediaItem]:
    """Return a single best item: video preferred, otherwise largest image."""
    if not items:
        return []
    videos = [item for item in items if item.media_type == "video"]
    if videos:
        return [max(videos, key=lambda item: item.file_size)]
    return [max(items, key=lambda item: item.file_size)]


def _should_include_item(
    item: PinterestMedia,
    *,
    download_images: bool,
    download_videos: bool,
) -> bool:
    media_type = _media_type(item)
    if media_type == "video":
        return download_videos
    return download_images


def _to_media_item(
    item: PinterestMedia,
    source_url: str,
    file_path: Path,
) -> PinterestMediaItem:
    stat = file_path.stat()
    return PinterestMediaItem(
        source_url=source_url,
        media_type=_media_type(item),  # type: ignore[arg-type]
        title=item.alt,
        description=item.alt,
        original_media_url=item.video_stream.url if item.video_stream else item.src,
        file_path=str(file_path),
        file_size=stat.st_size,
        created_at=datetime.fromtimestamp(stat.st_mtime, tz=UTC).replace(tzinfo=None),
    )


def _create_client():
    if settings.pinterest_use_browser:
        return PinterestDL.with_browser(
            timeout=settings.pinterest_api_timeout_seconds,
            verbose=False,
        )
    return PinterestDL.with_api(
        timeout=settings.pinterest_api_timeout_seconds,
        verbose=False,
    )


def download_pinterest(
    url: str,
    output_dir: Path,
    *,
    max_items: int | None = None,
    download_images: bool | None = None,
    download_videos: bool | None = None,
) -> PinterestDownloadResult:
    """Scrape a Pinterest pin or board URL and download media to *output_dir*."""
    parsed = parse_pinterest_url(url)
    if not parsed:
        raise PinterestDownloadError(f"Invalid or unsupported Pinterest URL: {url}")

    limit = max_items if max_items is not None else settings.pinterest_max_items
    if parsed.url_type in _SINGLE_ITEM_URL_TYPES:
        limit = 1
    include_images = (
        download_images if download_images is not None else settings.pinterest_download_images
    )
    include_videos = (
        download_videos if download_videos is not None else settings.pinterest_download_videos
    )

    if not include_images and not include_videos:
        raise PinterestDownloadError("At least one of downloadImages or downloadVideos must be true")

    output_dir.mkdir(parents=True, exist_ok=True)
    result = PinterestDownloadResult(url=parsed.url, url_type=parsed.url_type)

    try:
        dl = _create_client()
        if settings.pinterest_cookies_path:
            dl = dl.with_cookies_path(settings.pinterest_cookies_path)

        if parsed.url_type in _SINGLE_ITEM_URL_TYPES:
            scraped = fetch_pin_media(
                parsed.url,
                dl,
                timeout=settings.pinterest_api_timeout_seconds,
            )
            medias = operations.download_media(
                scraped,
                output_dir,
                include_videos,
            )
            if settings.pinterest_save_metadata:
                operations.add_captions_to_meta(medias, verbose=False)
        else:
            medias = dl.scrape_and_download(
                url=parsed.url,
                output_dir=str(output_dir),
                num=limit,
                download_streams=include_videos,
                caption="metadata" if settings.pinterest_save_metadata else "none",
            )
    except PinterestNoMediaError:
        raise
    except PinterestDownloadError:
        raise
    except Exception as exc:
        logger.exception("pinterest-dl failed for %s", parsed.url)
        message = str(exc).strip() or exc.__class__.__name__
        if "private" in message.lower() or "403" in message or "401" in message:
            raise PinterestDownloadError(
                "This Pinterest content is private or requires authorized cookies"
            ) from exc
        raise PinterestDownloadError(f"Pinterest download failed: {message}") from exc

    if not medias:
        raise PinterestNoMediaError("No media found at this Pinterest URL")

    for item in medias:
        if not item.local_path or not item.local_path.is_file():
            continue
        if not _should_include_item(
            item,
            download_images=include_images,
            download_videos=include_videos,
        ):
            continue
        result.items.append(
            _to_media_item(item, source_url=parsed.url, file_path=item.local_path)
        )

    if not result.items:
        raise PinterestNoMediaError(
            "No matching media found for the requested image/video filters"
        )

    if parsed.url_type in _SINGLE_ITEM_URL_TYPES:
        result.items = _pick_primary_item(result.items)

    return result


def download_pinterest_paths(
    url: str,
    output_dir: Path,
    max_items: int = 10,
    download_streams: bool = True,
) -> list[Path]:
    """Backward-compatible helper returning only local file paths."""
    result = download_pinterest(
        url,
        output_dir,
        max_items=max_items,
        download_images=True,
        download_videos=download_streams,
    )
    return [Path(item.file_path) for item in result.items]

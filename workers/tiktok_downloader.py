import logging
import re
import shutil
import tempfile
import urllib.request
from dataclasses import dataclass, field
from enum import Enum
from pathlib import Path

import yt_dlp

from workers.downloader import build_ydl_opts

logger = logging.getLogger(__name__)

_WRITABLE_COOKIE_PATH = Path("/tmp/tiktok_cookies.txt")

_IMAGE_DOWNLOAD_TIMEOUT_SECONDS = 15
_VIDEO_EXTENSIONS = frozenset({".mp4", ".webm", ".mov", ".mkv", ".m4v"})
_IMAGE_EXTENSIONS = frozenset({".jpg", ".jpeg", ".png", ".webp"})
_AUDIO_EXTENSIONS = frozenset({".mp3", ".m4a", ".opus", ".aac", ".wav", ".flac", ".ogg"})


class TikTokPostType(str, Enum):
    VIDEO = "video"
    CAROUSEL = "carousel"
    AUDIO_ONLY = "audio_only"
    UNKNOWN = "unknown"


@dataclass
class TikTokDownloadResult:
    source_url: str
    resolved_url: str
    post_type: TikTokPostType
    title: str = ""
    author: str = ""
    description: str = ""
    images: list[str] = field(default_factory=list)
    audio: str | None = None
    video_path: str | None = None
    errors: list[str] = field(default_factory=list)
    carousel_images_available: bool = False
    carousel_image_count: int = 0


class TikTokDownloadError(Exception):
    """Raised when a TikTok URL cannot be downloaded."""


class TikTokNoMediaError(TikTokDownloadError):
    """Raised when yt-dlp succeeds but no downloadable media is found."""


def _build_tiktok_ydl_opts(
    output_dir: Path,
    format_id: str | None = None,
    **extra,
) -> dict:
    from bot.config import settings

    opts = build_ydl_opts(output_dir, format_id=format_id)
    cookies_path = settings.tiktok_cookies_path.strip()
    browser = settings.tiktok_cookies_from_browser.strip()
    cookiefile = _writable_tiktok_cookiefile(cookies_path)
    if cookiefile:
        opts["cookiefile"] = cookiefile
    elif browser:
        opts["cookiesfrombrowser"] = (browser,)
    opts.update(extra)
    return opts


def _writable_tiktok_cookiefile(cookies_path: str) -> str | None:
    """Copy read-only mounted cookie jar to /tmp so yt-dlp can update it."""
    if not cookies_path:
        return None
    src = Path(cookies_path)
    if not src.is_file():
        return None
    dst = _WRITABLE_COOKIE_PATH
    try:
        if not dst.exists() or src.stat().st_mtime > dst.stat().st_mtime:
            shutil.copy2(src, dst)
    except OSError:
        logger.warning("failed to copy TikTok cookies to %s", dst, exc_info=True)
        return str(src)
    return str(dst)


def _detect_post_type(task_dir: Path) -> TikTokPostType:
    """Detect post type by examining files in the download directory."""
    has_video = False
    has_image = False
    has_audio = False

    for path in task_dir.iterdir():
        if not path.is_file():
            continue
        suffix = path.suffix.lower()
        if suffix in _VIDEO_EXTENSIONS:
            has_video = True
        elif suffix in _IMAGE_EXTENSIONS:
            has_image = True
        elif suffix in _AUDIO_EXTENSIONS:
            has_audio = True

    if has_video:
        return TikTokPostType.VIDEO
    if has_image:
        return TikTokPostType.CAROUSEL
    if has_audio:
        return TikTokPostType.AUDIO_ONLY
    return TikTokPostType.UNKNOWN


def _resolve_url(url: str) -> tuple[str, dict | None]:
    """Resolve a short TikTok URL and return (resolved_url, info_dict)."""
    page_url = resolve_tiktok_page_url(url)
    extract_url = canonical_tiktok_video_url(page_url)
    extract_opts = {
        **_build_tiktok_ydl_opts(Path("/tmp"), format_id=None),
        "skip_download": True,
        "extract_flat": False,
        "quiet": True,
        "no_warnings": True,
    }
    try:
        with yt_dlp.YoutubeDL(extract_opts) as ydl:
            info = ydl.extract_info(extract_url, download=False)
        if not info:
            return page_url, None

        resolved = (
            info.get("webpage_url")
            or info.get("original_url")
            or extract_url
        )
        return resolved, info
    except Exception:
        logger.exception("failed to resolve TikTok URL: %s", url)
        return page_url, None


_TIKTOK_FALLBACK_TITLE_RE = re.compile(r"^TikTok\s+video\s+#\d+\s*$", re.IGNORECASE)
_TIKTOK_PAGE_ITEM_RE = re.compile(
    r"https?://(?:www\.)?tiktok\.com/@([\w.-]*)/(photo|video)/(\d+)",
    re.IGNORECASE,
)
_TIKTOK_ITEM_ID_RE = re.compile(
    r"/(?:photo|video)/(\d+)",
    re.IGNORECASE,
)
_TIKTOK_USER_AGENT = (
    "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 "
    "(KHTML, like Gecko) Chrome/120.0.0.0 Safari/537.36"
)


def canonical_tiktok_video_url(url: str) -> str:
    """Map /photo/ URLs to /video/ so yt-dlp and web extraction can handle them."""
    match = _TIKTOK_PAGE_ITEM_RE.search(url)
    if match:
        user, _, item_id = match.groups()
        handle = user or ""
        return f"https://www.tiktok.com/@{handle}/video/{item_id}"
    item_match = _TIKTOK_ITEM_ID_RE.search(url)
    if item_match:
        item_id = item_match.group(1)
        return f"https://www.tiktok.com/@/video/{item_id}"
    return url.split("?", 1)[0]


def _follow_tiktok_redirect(url: str) -> str:
    request = urllib.request.Request(url, headers={"User-Agent": _TIKTOK_USER_AGENT})
    with urllib.request.urlopen(request, timeout=_IMAGE_DOWNLOAD_TIMEOUT_SECONDS) as response:
        return response.geturl()


def resolve_tiktok_page_url(url: str) -> str:
    """Resolve short links and normalize photo posts to canonical video page URLs."""
    if _TIKTOK_PAGE_ITEM_RE.search(url):
        return canonical_tiktok_video_url(url)
    return canonical_tiktok_video_url(_follow_tiktok_redirect(url))


def _extract_web_item_struct(page_url: str) -> dict | None:
    """Load TikTok itemStruct from embedded web data (works for photo carousels)."""
    video_url = canonical_tiktok_video_url(page_url)
    item_id_match = _TIKTOK_ITEM_ID_RE.search(video_url)
    if not item_id_match:
        return None
    item_id = item_id_match.group(1)
    extract_opts = {
        **_build_tiktok_ydl_opts(Path("/tmp"), format_id=None),
        "skip_download": True,
        "quiet": True,
        "no_warnings": True,
    }
    try:
        with yt_dlp.YoutubeDL(extract_opts) as ydl:
            ie = ydl.get_info_extractor("TikTok")
            item, _status = ie._extract_web_data_and_status(
                video_url, item_id, fatal=False,
            )
        return item if item else None
    except Exception:
        logger.warning(
            "failed to extract TikTok web item from %s",
            page_url,
            exc_info=True,
        )
        return None


def _extract_image_post_urls(item: dict) -> list[str]:
    images = (item.get("imagePost") or {}).get("images") or []
    image_urls: list[str] = []
    for image in images:
        if not image:
            continue
        url_list = (image.get("imageURL") or {}).get("urlList") or []
        if not url_list:
            url_list = (image.get("displayImage") or {}).get("urlList") or []
        if not url_list:
            continue
        picked = url_list[0]
        for candidate in url_list:
            lower = candidate.lower()
            if ".jpeg" in lower or ".jpg" in lower:
                picked = candidate
                break
        if picked not in image_urls:
            image_urls.append(picked)
    return image_urls


def _metadata_from_web_item(item: dict) -> tuple[str, str, str]:
    description = item.get("desc") or ""
    author = (item.get("author") or {}).get("uniqueId") or ""
    title = _normalize_tiktok_title("", description)
    return title, author, description


def _download_photo_carousel(
    url: str,
    output_dir: Path,
    *,
    max_images: int = 0,
    audio_enabled: bool = True,
) -> TikTokDownloadResult | None:
    page_url = resolve_tiktok_page_url(url)
    web_item = _extract_web_item_struct(page_url)
    if not web_item:
        return None

    photo_image_urls = _extract_image_post_urls(web_item)
    if not photo_image_urls:
        return None

    title, author, description = _metadata_from_web_item(web_item)
    result = TikTokDownloadResult(
        source_url=url,
        resolved_url=page_url,
        post_type=TikTokPostType.UNKNOWN,
        title=title,
        author=author,
        description=description,
        carousel_image_count=len(photo_image_urls),
    )
    result.images = _download_carousel_images(
        photo_image_urls, output_dir, max_images=max_images,
    )
    if not result.images:
        result.errors.append("no downloadable media found")
        return result

    result.post_type = TikTokPostType.CAROUSEL
    if audio_enabled:
        audio_path = _download_audio_from_entry(
            canonical_tiktok_video_url(page_url), output_dir,
        )
        if audio_path:
            result.audio = str(audio_path)
    return result


def _normalize_tiktok_title(title: str, description: str) -> str:
    desc = description.strip()
    if desc:
        return desc
    tit = title.strip()
    if not tit or _TIKTOK_FALLBACK_TITLE_RE.match(tit):
        return ""
    return tit


def _extract_metadata(info: dict) -> tuple[str, str, str]:
    """Extract title, author, description from yt-dlp info dict."""
    raw_title = info.get("title") or ""
    author = info.get("uploader") or info.get("creator") or ""
    description = info.get("description") or ""
    title = _normalize_tiktok_title(raw_title, description)
    return title, author, description


def _find_media_files(task_dir: Path) -> tuple[list[Path], Path | None, Path | None]:
    """Find images, video, and audio files in the download directory."""
    images: list[Path] = []
    video: Path | None = None
    audio: Path | None = None

    for path in sorted(task_dir.iterdir()):
        if not path.is_file():
            continue
        suffix = path.suffix.lower()
        if suffix in _IMAGE_EXTENSIONS:
            images.append(path)
        elif suffix in _VIDEO_EXTENSIONS:
            video = path
        elif suffix in _AUDIO_EXTENSIONS:
            audio = path

    return images, video, audio


def _download_image(
    url: str,
    output_path: Path,
    *,
    timeout: int = _IMAGE_DOWNLOAD_TIMEOUT_SECONDS,
) -> bool:
    """Download a single image from URL to the given path."""
    try:
        with urllib.request.urlopen(url, timeout=timeout) as response:
            with output_path.open("wb") as output:
                shutil.copyfileobj(response, output)
        return True
    except Exception as exc:
        logger.warning("failed to download image from %s: %s", url, exc)
        return False


def _looks_like_video_url(url: str) -> bool:
    lower = url.lower()
    if "/video/" in lower:
        return True
    suffix = Path(url.split("?", 1)[0]).suffix.lower()
    return suffix in _VIDEO_EXTENSIONS


def _is_multi_slide_carousel(info: dict) -> bool:
    entries = info.get("entries") or []
    return bool(info.get("is_slideshow")) or len(entries) >= 2


def _carousel_images_available(info: dict, image_urls: list[str]) -> bool:
    return _is_multi_slide_carousel(info) and len(image_urls) >= 2


def _extract_carousel_image_urls(info: dict) -> list[str]:
    """Extract deduplicated image URLs from yt-dlp carousel/slideshow info."""
    entries = info.get("entries") or []
    image_urls: list[str] = []
    for entry in entries:
        if not entry:
            continue
        thumbnails = entry.get("thumbnails") or []
        thumb_url = entry.get("thumbnail")
        if not thumb_url and thumbnails:
            thumb_url = thumbnails[0].get("url")
        if not thumb_url:
            entry_url = entry.get("url") or ""
            if entry_url and not _looks_like_video_url(entry_url):
                thumb_url = entry_url
        if (
            thumb_url
            and not _looks_like_video_url(thumb_url)
            and thumb_url not in image_urls
        ):
            image_urls.append(thumb_url)
    return image_urls


def _download_carousel_images(
    image_urls: list[str],
    output_dir: Path,
    *,
    max_images: int = 0,
) -> list[str]:
    """Download carousel image URLs to output_dir and return local paths."""
    if max_images > 0:
        image_urls = image_urls[:max_images]

    downloaded: list[str] = []
    for i, img_url in enumerate(image_urls):
        ext = _guess_extension(img_url)
        img_path = output_dir / f"image_{i:04d}{ext}"
        if _download_image(img_url, img_path):
            downloaded.append(str(img_path))
    return downloaded


def _download_audio_from_entry(
    url: str,
    output_dir: Path,
    format_id: str = "bestaudio/best",
) -> Path | None:
    """Download audio from a TikTok URL using yt-dlp with bestaudio format."""
    audio_opts = {
        **_build_tiktok_ydl_opts(output_dir, format_id=format_id),
        "format": format_id,
        "postprocessors": [
            {
                "key": "FFmpegExtractAudio",
                "preferredcodec": "mp3",
            }
        ],
    }
    try:
        with yt_dlp.YoutubeDL(audio_opts) as ydl:
            ydl.extract_info(url, download=True)
        for path in output_dir.iterdir():
            if path.suffix.lower() in _AUDIO_EXTENSIONS:
                return path
    except Exception as exc:
        logger.warning("failed to download audio from %s: %s", url, exc)
    return None


def _entries_contain_video(info: dict) -> bool:
    for entry in info.get("entries") or []:
        if not entry:
            continue
        vcodec = entry.get("vcodec")
        if vcodec and vcodec != "none":
            return True
        url = entry.get("url") or ""
        if Path(url.split("?", 1)[0]).suffix.lower() in _VIDEO_EXTENSIONS:
            return True
    return False


def _prefer_video_delivery(info: dict) -> bool:
    """True when slideshow metadata should deliver video, not auto-sent photos."""
    if info.get("url"):
        return True
    return _entries_contain_video(info)


def download_tiktok(
    url: str,
    output_dir: Path,
    *,
    format_id: str = "best",
    max_images: int = 0,
    audio_enabled: bool = True,
) -> TikTokDownloadResult:
    """Download a TikTok post (video or carousel) using yt-dlp.

    Returns structured result with post type detection and media paths.
    """
    output_dir.mkdir(parents=True, exist_ok=True)

    photo_result = _download_photo_carousel(
        url, output_dir, max_images=max_images, audio_enabled=audio_enabled,
    )
    if photo_result is not None:
        if photo_result.images or photo_result.audio:
            return photo_result
        if photo_result.errors:
            return photo_result

    # Step 1: Resolve URL and extract info
    resolved_url, info = _resolve_url(url)
    if not info:
        raise TikTokNoMediaError("No info returned from yt-dlp")

    title, author, description = _extract_metadata(info)
    result = TikTokDownloadResult(
        source_url=url,
        resolved_url=resolved_url,
        post_type=TikTokPostType.UNKNOWN,
        title=title,
        author=author,
        description=description,
    )

    # Step 2: Determine post type
    image_urls = _extract_carousel_image_urls(info)
    is_slideshow = bool(info.get("is_slideshow")) or bool(info.get("entries"))
    prefer_video = is_slideshow and _prefer_video_delivery(info)

    if is_slideshow and image_urls and not prefer_video:
        # Carousel/slideshow — download images and audio separately
        result.images = _download_carousel_images(
            image_urls, output_dir, max_images=max_images,
        )

        if result.images:
            result.post_type = TikTokPostType.CAROUSEL

            if audio_enabled:
                audio_path = _download_audio_from_entry(url, output_dir)
                if audio_path:
                    result.audio = str(audio_path)

            # If images downloaded but no audio — that's fine
            return result

        # No images downloaded — try audio-only
        if not audio_enabled:
            result.post_type = TikTokPostType.UNKNOWN
            result.errors.append("no downloadable media found")
            return result

        try:
            audio_path = _download_audio_from_entry(url, output_dir)
            if audio_path:
                result.audio = str(audio_path)
                result.post_type = TikTokPostType.AUDIO_ONLY
                return result
        except Exception:
            pass

        result.post_type = TikTokPostType.UNKNOWN
        result.errors.append("no downloadable media found")
        return result

    # Regular video/audio post — download the actual file
    try:
        download_opts = _build_tiktok_ydl_opts(output_dir, format_id=format_id)
        with yt_dlp.YoutubeDL(download_opts) as ydl:
            ydl.extract_info(url, download=True)
    except Exception as exc:
        raise TikTokDownloadError(f"Failed to download TikTok media: {exc}") from exc

    images, video_path, audio_path = _find_media_files(output_dir)
    result.images = [str(p) for p in images]
    result.video_path = str(video_path) if video_path else None
    result.audio = str(audio_path) if audio_path else None
    result.post_type = _detect_post_type(output_dir)

    carousel_image_urls = image_urls
    if _carousel_images_available(info, carousel_image_urls):
        result.carousel_images_available = True
        result.carousel_image_count = len(carousel_image_urls)

    return result


def download_tiktok_carousel_images(
    url: str,
    output_dir: Path,
    *,
    max_images: int = 0,
) -> TikTokDownloadResult:
    """Download only carousel images from a TikTok post."""
    output_dir.mkdir(parents=True, exist_ok=True)

    photo_result = _download_photo_carousel(
        url, output_dir, max_images=max_images, audio_enabled=False,
    )
    if photo_result is not None and photo_result.images:
        photo_result.post_type = TikTokPostType.CAROUSEL
        return photo_result

    resolved_url, info = _resolve_url(url)
    if not info:
        raise TikTokNoMediaError("No info returned from yt-dlp")

    title, author, description = _extract_metadata(info)
    result = TikTokDownloadResult(
        source_url=url,
        resolved_url=resolved_url,
        post_type=TikTokPostType.UNKNOWN,
        title=title,
        author=author,
        description=description,
    )

    image_urls = _extract_carousel_image_urls(info)
    result.carousel_image_count = len(image_urls)
    result.images = _download_carousel_images(
        image_urls, output_dir, max_images=max_images,
    )

    if result.images:
        result.post_type = TikTokPostType.CAROUSEL
        return result

    result.errors.append("no downloadable media found")
    return result


def _persist_tiktok_cookies(cookies_path: str) -> None:
    if not cookies_path or not _WRITABLE_COOKIE_PATH.is_file():
        return
    try:
        shutil.copy2(_WRITABLE_COOKIE_PATH, Path(cookies_path))
    except OSError:
        logger.warning(
            "failed to persist TikTok cookies to %s",
            cookies_path,
            exc_info=True,
        )


def refresh_tiktok_session(probe_url: str) -> bool:
    """Download a probe video to keep TikTok cookies/session warm."""
    from bot.config import settings

    cookies_path = settings.tiktok_cookies_path.strip()
    browser = settings.tiktok_cookies_from_browser.strip()
    if not cookies_path and not browser:
        return False

    if cookies_path and not _writable_tiktok_cookiefile(cookies_path) and not browser:
        logger.warning("tiktok cookie refresh skipped: cookie file missing")
        return False

    page_url = resolve_tiktok_page_url(probe_url)
    extract_url = canonical_tiktok_video_url(page_url)
    try:
        with tempfile.TemporaryDirectory() as tmp:
            output_dir = Path(tmp)
            opts = _build_tiktok_ydl_opts(output_dir, format_id="best")
            with yt_dlp.YoutubeDL(opts) as ydl:
                ydl.extract_info(extract_url, download=True)
    except Exception:
        logger.exception("tiktok cookie refresh failed", extra={"url": probe_url})
        return False

    _persist_tiktok_cookies(cookies_path)
    logger.info("tiktok cookie refresh ok", extra={"url": probe_url})
    return True


def _guess_extension(url: str) -> str:
    """Guess file extension from a URL."""
    from urllib.parse import urlparse, unquote

    path = unquote(urlparse(url).path)
    suffix = Path(path).suffix.lower()
    if suffix in (".jpg", ".jpeg", ".png", ".webp"):
        return suffix
    if suffix:
        return suffix
    return ".jpg"

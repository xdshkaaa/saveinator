import logging
import shutil
import urllib.request
from dataclasses import dataclass, field
from enum import Enum
from pathlib import Path

import yt_dlp

from workers.downloader import build_ydl_opts

logger = logging.getLogger(__name__)

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


class TikTokDownloadError(Exception):
    """Raised when a TikTok URL cannot be downloaded."""


class TikTokNoMediaError(TikTokDownloadError):
    """Raised when yt-dlp succeeds but no downloadable media is found."""


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
    extract_opts = {
        **build_ydl_opts(Path("/tmp"), format_id=None),
        "skip_download": True,
        "extract_flat": False,
        "quiet": True,
        "no_warnings": True,
    }
    try:
        with yt_dlp.YoutubeDL(extract_opts) as ydl:
            info = ydl.extract_info(url, download=False)
        if not info:
            return url, None

        resolved = (
            info.get("url")
            or info.get("webpage_url")
            or info.get("original_url")
            or url
        )
        return resolved, info
    except Exception:
        logger.exception("failed to resolve TikTok URL", url=url)
        return url, None


def _extract_metadata(info: dict) -> tuple[str, str, str]:
    """Extract title, author, description from yt-dlp info dict."""
    title = info.get("title") or ""
    author = info.get("uploader") or info.get("creator") or ""
    description = info.get("description") or ""
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


def _download_image(url: str, output_path: Path) -> bool:
    """Download a single image from URL to the given path."""
    try:
        urllib.request.urlretrieve(url, output_path)
        return True
    except Exception:
        logger.warning("failed to download image", url=url)
        return False


def _download_audio_from_entry(
    url: str,
    output_dir: Path,
    format_id: str = "bestaudio/best",
) -> Path | None:
    """Download audio from a TikTok URL using yt-dlp with bestaudio format."""
    audio_opts = {
        **build_ydl_opts(output_dir, format_id=format_id),
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
    except Exception:
        logger.warning("failed to download audio", url=url)
    return None


def download_tiktok(
    url: str,
    output_dir: Path,
    *,
    format_id: str = "best",
    max_images: int = 0,
) -> TikTokDownloadResult:
    """Download a TikTok post (video or carousel) using yt-dlp.

    Returns structured result with post type detection and media paths.
    """
    output_dir.mkdir(parents=True, exist_ok=True)

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
    is_slideshow = bool(info.get("is_slideshow")) or bool(info.get("entries"))

    if is_slideshow:
        # Carousel/slideshow — download images and audio separately
        entries = info.get("entries") or []
        image_urls: list[str] = []
        audio_url = info.get("url") or info.get("audio_url") or None

        for entry in entries:
            if not entry:
                continue
            thumb_url = (
                entry.get("thumbnail")
                or entry.get("url")
                or entry.get("thumbnails", [{}])[0].get("url")
                if entry.get("thumbnails")
                else None
            )
            if thumb_url and thumb_url not in image_urls:
                image_urls.append(thumb_url)

        if max_images > 0:
            image_urls = image_urls[:max_images]

        # Download each image
        for i, img_url in enumerate(image_urls):
            ext = _guess_extension(img_url)
            img_path = output_dir / f"image_{i:04d}{ext}"
            if _download_image(img_url, img_path):
                result.images.append(str(img_path))

        if result.images:
            result.post_type = TikTokPostType.CAROUSEL

            # Try to download audio for carousel
            try:
                audio_path = _download_audio_from_entry(url, output_dir)
                if audio_path:
                    result.audio = str(audio_path)
            except Exception:
                pass

            # If images downloaded but no audio — that's fine
            return result

        # No images downloaded — try audio-only
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
        download_opts = build_ydl_opts(output_dir, format_id=format_id)
        with yt_dlp.YoutubeDL(download_opts) as ydl:
            ydl.extract_info(url, download=True)
    except Exception as exc:
        raise TikTokDownloadError(f"Failed to download TikTok media: {exc}") from exc

    images, video_path, audio_path = _find_media_files(output_dir)
    result.images = [str(p) for p in images]
    result.video_path = str(video_path) if video_path else None
    result.audio = str(audio_path) if audio_path else None
    result.post_type = _detect_post_type(output_dir)

    return result


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

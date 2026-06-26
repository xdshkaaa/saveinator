import time
from pathlib import Path

import structlog
import yt_dlp

from bot.config import Settings
from bot.services.runtime_settings import spotify_track_timeout_seconds
from bot.services.spotify_models import NormalizedSpotifyTrack

logger = structlog.get_logger()

AUDIO_EXTENSIONS = {".mp3", ".m4a", ".flac", ".wav", ".aac", ".opus", ".webm", ".ogg"}


class YoutubeAudioError(Exception):
    pass


class YoutubeAudioNotFoundError(YoutubeAudioError):
    pass


class YoutubeAudioTimeoutError(YoutubeAudioError):
    pass


def is_yt_dlp_available() -> bool:
    try:
        import yt_dlp  # noqa: F401
    except ImportError:
        return False
    return True


def is_spotify_download_enabled(settings: Settings) -> bool:
    return settings.spotify_download_enabled and is_yt_dlp_available()


def build_track_search_query(track: NormalizedSpotifyTrack) -> str:
    return f"{track.artists} - {track.title}"


def youtube_watch_url(video_id: str) -> str:
    return f"https://youtube.com/watch?v={video_id}"


def _base_ydl_opts() -> dict:
    return {
        "quiet": True,
        "no_warnings": True,
        "noplaylist": True,
        "ignoreerrors": False,
        "js_runtimes": {"deno": {}},
        "remote_components": {"ejs:github"},
        "socket_timeout": spotify_track_timeout_seconds(),
    }


def _search_opts() -> dict:
    return {
        **_base_ydl_opts(),
        "skip_download": True,
    }


def _download_opts(output_dir: Path, settings: Settings) -> dict:
    return {
        **_base_ydl_opts(),
        "postprocessor_args": {"ffmpeg": ["-threads", "1"]},
        "outtmpl": str(output_dir / "%(title).100s.%(ext)s"),
        "format": "bestaudio/best",
        "postprocessors": [
            {
                "key": "FFmpegExtractAudio",
                "preferredcodec": settings.spotify_dl_output_format,
                "preferredquality": "0",
            }
        ],
    }


def _find_audio_file(output_dir: Path) -> Path | None:
    for path in sorted(output_dir.iterdir(), key=lambda p: p.stat().st_mtime, reverse=True):
        if path.is_file() and path.suffix.lower() in AUDIO_EXTENSIONS:
            return path
    return None


def _video_id_from_search_info(info: dict | None) -> str | None:
    if not info:
        return None
    entries = info.get("entries") or []
    if entries:
        first = entries[0]
        if first and first.get("id"):
            return str(first["id"])
    if info.get("id"):
        return str(info["id"])
    return None


def resolve_youtube_video_id(query: str, settings: Settings) -> str | None:
    """Resolve the first ytsearch result ID without downloading audio."""
    if not is_yt_dlp_available():
        return None

    timeout = spotify_track_timeout_seconds()
    started = time.monotonic()
    try:
        with yt_dlp.YoutubeDL(_search_opts()) as ydl:
            info = ydl.extract_info(f"ytsearch1:{query}", download=False)
    except yt_dlp.utils.DownloadError as exc:
        detail = str(exc)
        if "timed out" in detail.lower() or time.monotonic() - started > timeout:
            logger.warning("youtube search resolve timed out", query=query)
            return None
        logger.warning("youtube search resolve failed", query=query, detail=detail[:500])
        return None
    except Exception as exc:
        if time.monotonic() - started > timeout:
            logger.warning("youtube search resolve timed out", query=query)
            return None
        logger.warning("youtube search resolve failed", query=query, detail=str(exc)[:500])
        return None

    return _video_id_from_search_info(info)


def download_track_from_youtube(
    track: NormalizedSpotifyTrack,
    output_dir: Path,
    settings: Settings,
    *,
    youtube_url: str,
) -> Path:
    if not is_yt_dlp_available():
        raise YoutubeAudioNotFoundError("yt-dlp is not installed")

    if not youtube_url:
        raise YoutubeAudioError("YouTube URL is required")

    output_dir.mkdir(parents=True, exist_ok=True)
    query = build_track_search_query(track)
    timeout = spotify_track_timeout_seconds()
    logger.info("youtube audio download", query=query, url=youtube_url)

    started = time.monotonic()
    try:
        with yt_dlp.YoutubeDL(_download_opts(output_dir, settings)) as ydl:
            ydl.download([youtube_url])
    except yt_dlp.utils.DownloadError as exc:
        detail = str(exc)
        if "timed out" in detail.lower():
            raise YoutubeAudioTimeoutError(
                f"Track download timed out after {timeout}s"
            ) from exc
        raise YoutubeAudioError(detail) from exc
    except Exception as exc:
        if time.monotonic() - started > timeout:
            raise YoutubeAudioTimeoutError(
                f"Track download timed out after {timeout}s"
            ) from exc
        raise YoutubeAudioError(str(exc)) from exc

    audio_path = _find_audio_file(output_dir)
    if audio_path is None:
        raise YoutubeAudioError(f"No audio file found for query: {query}")

    return audio_path

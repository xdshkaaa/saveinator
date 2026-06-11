import shutil
import subprocess
from pathlib import Path

import structlog

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
    return shutil.which("yt-dlp") is not None


def is_spotify_download_enabled(settings: Settings) -> bool:
    return settings.spotify_download_enabled and is_yt_dlp_available()


def build_track_search_query(track: NormalizedSpotifyTrack) -> str:
    return f"{track.artists} - {track.title}"


def youtube_watch_url(video_id: str) -> str:
    return f"https://youtube.com/watch?v={video_id}"


def resolve_youtube_video_id(query: str, settings: Settings) -> str | None:
    """Resolve the first ytsearch result ID without downloading audio."""
    if not is_yt_dlp_available():
        return None

    command = [
        "yt-dlp",
        f"ytsearch1:{query}",
        "--print",
        "id",
        "--skip-download",
        "--no-playlist",
        "--no-warnings",
        "--quiet",
    ]

    try:
        result = subprocess.run(
            command,
            capture_output=True,
            text=True,
            timeout=spotify_track_timeout_seconds(),
            check=False,
        )
    except subprocess.TimeoutExpired:
        logger.warning("youtube search resolve timed out", query=query)
        return None

    if result.returncode != 0:
        detail = (result.stderr or result.stdout or f"exit code {result.returncode}").strip()
        logger.warning("youtube search resolve failed", query=query, detail=detail[:500])
        return None

    video_id = result.stdout.strip().splitlines()[0] if result.stdout.strip() else ""
    return video_id or None


def download_track_from_youtube(
    track: NormalizedSpotifyTrack,
    output_dir: Path,
    settings: Settings,
    *,
    youtube_url: str | None = None,
) -> Path:
    if not is_yt_dlp_available():
        raise YoutubeAudioNotFoundError("yt-dlp is not installed")

    output_dir.mkdir(parents=True, exist_ok=True)
    query = build_track_search_query(track)
    target = youtube_url or f"ytsearch1:{query}"
    outtmpl = str(output_dir / "%(title).100s.%(ext)s")

    command = [
        "yt-dlp",
        target,
        "-x",
        "--audio-format",
        settings.spotify_dl_output_format,
        "--audio-quality",
        "0",
        "--no-playlist",
        "--no-warnings",
        "--quiet",
        "-o",
        outtmpl,
    ]

    logger.info("youtube audio download", query=query, cached_url=bool(youtube_url))

    try:
        result = subprocess.run(
            command,
            capture_output=True,
            text=True,
            timeout=spotify_track_timeout_seconds(),
            check=False,
        )
    except subprocess.TimeoutExpired as exc:
        track_timeout = spotify_track_timeout_seconds()
        raise YoutubeAudioTimeoutError(
            f"Track download timed out after {track_timeout}s"
        ) from exc

    if result.returncode != 0:
        detail = (result.stderr or result.stdout or f"exit code {result.returncode}").strip()
        logger.warning("yt-dlp failed", query=query, detail=detail[:500])
        raise YoutubeAudioError(detail)

    for path in sorted(output_dir.iterdir(), key=lambda p: p.stat().st_mtime, reverse=True):
        if path.is_file() and path.suffix.lower() in AUDIO_EXTENSIONS:
            return path

    raise YoutubeAudioError(f"No audio file found for query: {query}")

import shutil
import subprocess
from pathlib import Path

import structlog

from bot.config import Settings
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


def download_track_from_youtube(
    track: NormalizedSpotifyTrack,
    output_dir: Path,
    settings: Settings,
) -> Path:
    if not is_yt_dlp_available():
        raise YoutubeAudioNotFoundError("yt-dlp is not installed")

    output_dir.mkdir(parents=True, exist_ok=True)
    query = build_track_search_query(track)
    outtmpl = str(output_dir / "%(title).100s.%(ext)s")

    command = [
        "yt-dlp",
        f"ytsearch1:{query}",
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

    logger.info("youtube audio search", query=query)

    try:
        result = subprocess.run(
            command,
            capture_output=True,
            text=True,
            timeout=settings.spotify_track_timeout_seconds,
            check=False,
        )
    except subprocess.TimeoutExpired as exc:
        raise YoutubeAudioTimeoutError(
            f"Track download timed out after {settings.spotify_track_timeout_seconds}s"
        ) from exc

    if result.returncode != 0:
        detail = (result.stderr or result.stdout or f"exit code {result.returncode}").strip()
        logger.warning("yt-dlp failed", query=query, detail=detail[:500])
        raise YoutubeAudioError(detail)

    for path in sorted(output_dir.iterdir(), key=lambda p: p.stat().st_mtime, reverse=True):
        if path.is_file() and path.suffix.lower() in AUDIO_EXTENSIONS:
            return path

    raise YoutubeAudioError(f"No audio file found for query: {query}")

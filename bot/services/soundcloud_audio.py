import shutil
import subprocess
from pathlib import Path

import structlog

from bot.config import Settings
from bot.metrics import (
    SOUNDCLOUD_DOWNLOAD_DURATION_SECONDS,
    SOUNDCLOUD_DOWNLOAD_FAILURES_TOTAL,
    SOUNDCLOUD_DOWNLOADS_SUCCESS_TOTAL,
    SOUNDCLOUD_DOWNLOADS_TIMEOUT_TOTAL,
)
from bot.services.runtime_settings import soundcloud_max_file_mb, soundcloud_track_timeout_seconds
from bot.services.soundcloud_models import NormalizedSoundCloudTrack

logger = structlog.get_logger()

AUDIO_EXTENSIONS = {".mp3", ".m4a", ".flac", ".wav", ".aac", ".opus", ".webm", ".ogg"}


class SoundCloudAudioError(Exception):
    pass


class SoundCloudAudioNotFoundError(SoundCloudAudioError):
    pass


class SoundCloudAudioTimeoutError(SoundCloudAudioError):
    pass


class SoundCloudAudioTooLargeError(SoundCloudAudioError):
    pass


def is_yt_dlp_available() -> bool:
    if shutil.which("yt-dlp") is not None:
        return True
    try:
        import yt_dlp  # noqa: F401
    except ImportError:
        return False
    return True


def is_soundcloud_download_enabled(settings: Settings) -> bool:
    return settings.soundcloud_download_enabled and is_yt_dlp_available()


def download_track(
    track: NormalizedSoundCloudTrack,
    output_dir: Path,
    settings: Settings,
) -> Path:
    if not is_yt_dlp_available():
        raise SoundCloudAudioNotFoundError("yt-dlp is not installed")

    output_dir.mkdir(parents=True, exist_ok=True)
    outtmpl = str(output_dir / "%(title).100s.%(ext)s")
    command = [
        "yt-dlp",
        track.soundcloud_url,
        "-x",
        "--audio-format",
        settings.soundcloud_dl_output_format,
        "--audio-quality",
        "0",
        "--no-playlist",
        "--no-warnings",
        "--quiet",
        "-o",
        outtmpl,
    ]

    timeout = soundcloud_track_timeout_seconds()
    logger.info("soundcloud audio download", url=track.soundcloud_url, title=track.title)

    import time

    started = time.monotonic()
    try:
        result = subprocess.run(
            command,
            capture_output=True,
            text=True,
            timeout=timeout,
            check=False,
        )
    except subprocess.TimeoutExpired as exc:
        SOUNDCLOUD_DOWNLOADS_TIMEOUT_TOTAL.inc()
        SOUNDCLOUD_DOWNLOAD_FAILURES_TOTAL.inc()
        raise SoundCloudAudioTimeoutError(
            f"Track download timed out after {timeout}s"
        ) from exc

    if result.returncode != 0:
        detail = (result.stderr or result.stdout or f"exit code {result.returncode}").strip()
        logger.warning("soundcloud yt-dlp failed", url=track.soundcloud_url, detail=detail[:500])
        SOUNDCLOUD_DOWNLOAD_FAILURES_TOTAL.inc()
        raise SoundCloudAudioError(detail)

    for path in sorted(output_dir.iterdir(), key=lambda p: p.stat().st_mtime, reverse=True):
        if path.is_file() and path.suffix.lower() in AUDIO_EXTENSIONS:
            max_bytes = soundcloud_max_file_mb() * 1024 * 1024
            if path.stat().st_size > max_bytes:
                SOUNDCLOUD_DOWNLOAD_FAILURES_TOTAL.inc()
                raise SoundCloudAudioTooLargeError(
                    f"Audio file exceeds {soundcloud_max_file_mb()} MB limit"
                )
            SOUNDCLOUD_DOWNLOADS_SUCCESS_TOTAL.inc()
            SOUNDCLOUD_DOWNLOAD_DURATION_SECONDS.observe(time.monotonic() - started)
            return path

    SOUNDCLOUD_DOWNLOAD_FAILURES_TOTAL.inc()
    raise SoundCloudAudioError(f"No audio file found for: {track.soundcloud_url}")

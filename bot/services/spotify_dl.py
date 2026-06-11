import shutil
import subprocess
from dataclasses import dataclass
from pathlib import Path

import structlog

from bot.config import Settings

logger = structlog.get_logger()

AUDIO_EXTENSIONS = {".mp3", ".m4a", ".flac", ".wav", ".aac", ".opus"}


class SpotifyDlError(Exception):
    pass


class SpotifyDlNotFoundError(SpotifyDlError):
    pass


class SpotifyDlTimeoutError(SpotifyDlError):
    pass


@dataclass
class SpotifyDlTrack:
    path: Path
    title: str


def is_spotify_dl_available(settings: Settings | None = None) -> bool:
    bin_name = settings.spotify_dl_bin if settings else "spotifydl"
    return shutil.which(bin_name) is not None


def is_spotify_download_enabled(settings: Settings) -> bool:
    return (
        settings.spotify_download_enabled
        and settings.spotify_client_id != ""
        and settings.spotify_client_secret != ""
        and is_spotify_dl_available(settings)
    )


def build_spotify_dl_command(
    spotify_url: str,
    output_dir: Path,
    settings: Settings,
) -> list[str]:
    app_key = f"{settings.spotify_client_id}:{settings.spotify_client_secret}"
    return [
        settings.spotify_dl_bin,
        spotify_url,
        "--o",
        str(output_dir),
        "--oo",
        "--ak",
        app_key,
        "--oft",
        settings.spotify_dl_output_format,
    ]


def collect_audio_files(output_dir: Path) -> list[Path]:
    files: list[Path] = []
    for path in output_dir.rglob("*"):
        if path.is_file() and path.suffix.lower() in AUDIO_EXTENSIONS:
            files.append(path)
    return sorted(files, key=lambda item: item.name.lower())


def run_spotify_dl(spotify_url: str, output_dir: Path, settings: Settings) -> list[SpotifyDlTrack]:
    if not is_spotify_dl_available(settings):
        raise SpotifyDlNotFoundError(f"{settings.spotify_dl_bin} is not installed")

    if not settings.spotify_client_id or not settings.spotify_client_secret:
        raise SpotifyDlError("Spotify credentials are not configured")

    output_dir.mkdir(parents=True, exist_ok=True)
    command = build_spotify_dl_command(spotify_url, output_dir, settings)

    logger.info("starting spotify-dl", url=spotify_url, output_dir=str(output_dir))

    try:
        result = subprocess.run(
            command,
            capture_output=True,
            text=True,
            timeout=settings.spotify_dl_timeout_seconds,
            check=False,
        )
    except subprocess.TimeoutExpired as exc:
        raise SpotifyDlTimeoutError(
            f"spotify-dl timed out after {settings.spotify_dl_timeout_seconds}s"
        ) from exc

    if result.returncode != 0:
        stderr = (result.stderr or "").strip()
        stdout = (result.stdout or "").strip()
        detail = stderr or stdout or f"exit code {result.returncode}"
        logger.warning("spotify-dl failed", returncode=result.returncode, detail=detail[:500])
        raise SpotifyDlError(detail)

    audio_files = collect_audio_files(output_dir)
    if not audio_files:
        raise SpotifyDlError("spotify-dl finished but no audio files were found")

    return [SpotifyDlTrack(path=path, title=path.stem) for path in audio_files]

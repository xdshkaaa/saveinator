import asyncio
from typing import Any

import structlog
import yt_dlp

from bot.config import Settings
from bot.metrics import SOUNDCLOUD_METADATA_FAILURES_TOTAL, SOUNDCLOUD_PLAYLIST_TRACKS
from bot.services.runtime_settings import soundcloud_track_timeout_seconds
from bot.services.soundcloud_cache import get_cached_release, set_cached_release
from bot.services.soundcloud_models import (
    NormalizedSoundCloudRelease,
    NormalizedSoundCloudTrack,
)
from bot.services.soundcloud_parser import SoundCloudLink

logger = structlog.get_logger()


class SoundCloudError(Exception):
    pass


class SoundCloudNotFoundError(SoundCloudError):
    pass


class SoundCloudTimeoutError(SoundCloudError):
    pass


class SoundCloudPlaylistTooLargeError(SoundCloudError):
    pass


class SoundCloudUnavailableError(SoundCloudError):
    pass


def _base_ydl_opts() -> dict[str, Any]:
    return {
        "quiet": True,
        "no_warnings": True,
        "skip_download": True,
        "ignoreerrors": False,
        "js_runtimes": {"deno": {}},
        "remote_components": {"ejs:github"},
    }


def _duration_ms(raw: float | int | None) -> int:
    if not raw:
        return 0
    return int(float(raw) * 1000)


def _best_thumbnail(info: dict[str, Any]) -> str | None:
    thumbnail = info.get("thumbnail")
    if thumbnail:
        return str(thumbnail)
    thumbnails = info.get("thumbnails") or []
    if thumbnails:
        return str(thumbnails[-1].get("url") or "")
    return None


def _artist(info: dict[str, Any]) -> str:
    return str(info.get("uploader") or info.get("artist") or info.get("creator") or "")


def _genre(info: dict[str, Any]) -> str:
    genre = info.get("genre")
    if genre:
        return str(genre)
    tags = info.get("tags")
    if isinstance(tags, list) and tags:
        return str(tags[0])
    return ""


def _normalize_track(info: dict[str, Any], *, track_number: int = 1) -> NormalizedSoundCloudTrack:
    source_id = str(info.get("id") or info.get("webpage_url_basename") or "")
    webpage_url = str(info.get("webpage_url") or info.get("url") or "")
    return NormalizedSoundCloudTrack(
        source_id=source_id,
        title=str(info.get("title") or ""),
        artist=_artist(info),
        duration_ms=_duration_ms(info.get("duration")),
        soundcloud_url=webpage_url,
        artwork_url=_best_thumbnail(info),
        genre=_genre(info),
        description=str(info.get("description") or ""),
        created_at=str(info.get("upload_date") or info.get("timestamp") or ""),
        track_number=track_number,
    )


def _is_playlist(info: dict[str, Any]) -> bool:
    if info.get("_type") in ("playlist", "multi_video"):
        return True
    entries = info.get("entries")
    return isinstance(entries, list) and len(entries) > 1


def _normalize_release(info: dict[str, Any], source_url: str) -> NormalizedSoundCloudRelease:
    entries = info.get("entries") or []
    valid_entries = [entry for entry in entries if entry]

    if _is_playlist(info) and valid_entries:
        tracks = [
            _normalize_track(entry, track_number=index)
            for index, entry in enumerate(valid_entries, start=1)
        ]
        release_type = "playlist"
    else:
        track_info = valid_entries[0] if valid_entries else info
        tracks = [_normalize_track(track_info, track_number=1)]
        release_type = "track"

    source_id = str(info.get("id") or source_url)
    artwork = _best_thumbnail(info)
    if not artwork and tracks:
        artwork = tracks[0].artwork_url

    return NormalizedSoundCloudRelease(
        source_id=source_id,
        title=str(info.get("title") or (tracks[0].title if tracks else "")),
        artist=_artist(info) or (tracks[0].artist if tracks else ""),
        release_type=release_type,
        artwork_url=artwork,
        soundcloud_url=str(info.get("webpage_url") or source_url),
        tracks=tracks,
    )


def _extract_metadata(url: str, settings: Settings) -> dict[str, Any]:
    opts = {
        **_base_ydl_opts(),
        "socket_timeout": settings.soundcloud_track_timeout_seconds,
    }
    try:
        with yt_dlp.YoutubeDL(opts) as ydl:
            return ydl.extract_info(url, download=False)
    except yt_dlp.utils.DownloadError as exc:
        detail = str(exc).lower()
        if "unavailable" in detail or "404" in detail or "not found" in detail:
            raise SoundCloudNotFoundError(str(exc)) from exc
        if "private" in detail:
            raise SoundCloudUnavailableError(str(exc)) from exc
        raise SoundCloudError(str(exc)) from exc
    except Exception as exc:
        raise SoundCloudError(str(exc)) from exc


async def fetch_release(link: SoundCloudLink, settings: Settings) -> NormalizedSoundCloudRelease:
    cached = await get_cached_release(link.url)
    if cached is not None:
        return cached

    timeout = soundcloud_track_timeout_seconds()
    try:
        info = await asyncio.wait_for(
            asyncio.to_thread(_extract_metadata, link.url, settings),
            timeout=timeout,
        )
    except asyncio.TimeoutError as exc:
        SOUNDCLOUD_METADATA_FAILURES_TOTAL.inc()
        raise SoundCloudTimeoutError(f"Metadata fetch timed out after {timeout}s") from exc
    except SoundCloudError:
        SOUNDCLOUD_METADATA_FAILURES_TOTAL.inc()
        raise

    if not info:
        SOUNDCLOUD_METADATA_FAILURES_TOTAL.inc()
        raise SoundCloudNotFoundError("No metadata returned")

    release = _normalize_release(info, link.url)

    if release.release_type == "playlist" and len(release.tracks) > settings.soundcloud_max_tracks:
        raise SoundCloudPlaylistTooLargeError(
            f"Playlist has {len(release.tracks)} tracks, limit is {settings.soundcloud_max_tracks}"
        )

    if release.release_type == "playlist":
        SOUNDCLOUD_PLAYLIST_TRACKS.observe(len(release.tracks))

    await set_cached_release(link.url, release, settings.soundcloud_meta_cache_ttl_seconds)
    return release

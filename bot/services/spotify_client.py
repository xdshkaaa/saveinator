import asyncio
import base64
import json
import time
from dataclasses import dataclass
from urllib.parse import urlencode

import httpx
import structlog

from bot.config import Settings
from bot.services.spotify_cache import get_cached_release, set_cached_release
from bot.services.release_metadata_store import persist_spotify_release
from bot.services.spotify_models import (
    NormalizedSpotifyRelease,
    normalize_album,
    release_from_track,
)

logger = structlog.get_logger()

TOKEN_URL = "https://accounts.spotify.com/api/token"
API_BASE = "https://api.spotify.com/v1"
MAX_RETRIES = 3


class SpotifyError(Exception):
    pass


class SpotifyAuthError(SpotifyError):
    pass


class SpotifyNotFoundError(SpotifyError):
    pass


class SpotifyRateLimitError(SpotifyError):
    pass


class SpotifyTimeoutError(SpotifyError):
    pass


class SpotifyApiError(SpotifyError):
    pass


@dataclass
class _TokenCache:
    access_token: str = ""
    expires_at: float = 0.0


_token_cache = _TokenCache()


def _basic_auth_header(client_id: str, client_secret: str) -> str:
    credentials = f"{client_id}:{client_secret}".encode()
    return "Basic " + base64.b64encode(credentials).decode("ascii")


async def _http_json(
    method: str,
    url: str,
    *,
    headers: dict[str, str] | None = None,
    data: bytes | None = None,
    timeout: float = 10.0,
) -> tuple[int, dict[str, str], dict | list]:
    try:
        async with httpx.AsyncClient(timeout=timeout) as client:
            response = await client.request(method, url, headers=headers, content=data)
    except httpx.TimeoutException as exc:
        raise SpotifyTimeoutError("Spotify API request timed out") from exc

    response_headers = {key.lower(): value for key, value in response.headers.items()}
    body = response.text
    if not body:
        return response.status_code, response_headers, {}

    try:
        payload: dict | list = json.loads(body)
    except json.JSONDecodeError:
        payload = {}
    return response.status_code, response_headers, payload


async def _request(
    method: str,
    url: str,
    *,
    headers: dict[str, str] | None = None,
    data: bytes | None = None,
    timeout: float = 10.0,
    attempt: int = 0,
) -> dict | list:
    status, response_headers, payload = await _http_json(
        method, url, headers=headers, data=data, timeout=timeout
    )

    if status == 429:
        if attempt >= MAX_RETRIES - 1:
            raise SpotifyRateLimitError("Spotify API rate limit exceeded")
        retry_after = response_headers.get("retry-after")
        delay = float(retry_after) if retry_after else 2 ** attempt
        logger.warning("spotify rate limited, retrying", status=status, delay=delay, attempt=attempt)
        await asyncio.sleep(delay)
        return await _request(
            method,
            url,
            headers=headers,
            data=data,
            timeout=timeout,
            attempt=attempt + 1,
        )

    if status in (401, 403):
        raise SpotifyAuthError(f"Spotify authentication failed with status {status}")
    if status == 404:
        raise SpotifyNotFoundError("Spotify resource not found")
    if status >= 400:
        raise SpotifyApiError(f"Spotify API error with status {status}")

    return payload


async def _get_access_token(settings: Settings) -> str:
    now = time.time()
    if _token_cache.access_token and now < _token_cache.expires_at - 30:
        return _token_cache.access_token

    if not settings.spotify_client_id or not settings.spotify_client_secret:
        raise SpotifyAuthError("Spotify credentials are not configured")

    body = urlencode({"grant_type": "client_credentials"}).encode()
    status, _, payload = await _http_json(
        "POST",
        TOKEN_URL,
        headers={
            "Authorization": _basic_auth_header(
                settings.spotify_client_id,
                settings.spotify_client_secret,
            ),
            "Content-Type": "application/x-www-form-urlencoded",
        },
        data=body,
        timeout=settings.spotify_api_timeout_seconds,
    )

    if status in (401, 403):
        raise SpotifyAuthError(f"Spotify token request failed with status {status}")
    if status == 429:
        raise SpotifyRateLimitError("Spotify token request rate limited")
    if status >= 400 or not isinstance(payload, dict):
        raise SpotifyApiError(f"Spotify token request failed with status {status}")

    access_token = payload.get("access_token")
    if not access_token:
        raise SpotifyAuthError("Spotify token response missing access_token")

    expires_in = int(payload.get("expires_in", 3600))
    _token_cache.access_token = access_token
    _token_cache.expires_at = now + expires_in
    return access_token


async def _authorized_headers(settings: Settings) -> dict[str, str]:
    token = await _get_access_token(settings)
    return {"Authorization": f"Bearer {token}"}


async def _fetch_album_tracks(settings: Settings, album_id: str) -> list[dict]:
    tracks: list[dict] = []
    offset = 0
    limit = 50

    while True:
        query = urlencode({"limit": limit, "offset": offset})
        url = f"{API_BASE}/albums/{album_id}/tracks?{query}"
        payload = await _request(
            "GET",
            url,
            headers=await _authorized_headers(settings),
            timeout=settings.spotify_api_timeout_seconds,
        )
        if not isinstance(payload, dict):
            raise SpotifyApiError("Unexpected Spotify tracks response")

        items = payload.get("items") or []
        tracks.extend(items)

        if not payload.get("next"):
            break
        offset += limit

    return tracks


async def fetch_album(album_id: str, settings: Settings) -> NormalizedSpotifyRelease:
    url = f"{API_BASE}/albums/{album_id}"
    album_payload = await _request(
        "GET",
        url,
        headers=await _authorized_headers(settings),
        timeout=settings.spotify_api_timeout_seconds,
    )
    if not isinstance(album_payload, dict):
        raise SpotifyApiError("Unexpected Spotify album response")

    tracks = await _fetch_album_tracks(settings, album_id)
    return normalize_album(album_payload, tracks)


async def fetch_track(track_id: str, settings: Settings) -> NormalizedSpotifyRelease:
    url = f"{API_BASE}/tracks/{track_id}"
    track_payload = await _request(
        "GET",
        url,
        headers=await _authorized_headers(settings),
        timeout=settings.spotify_api_timeout_seconds,
    )
    if not isinstance(track_payload, dict):
        raise SpotifyApiError("Unexpected Spotify track response")

    return release_from_track(track_payload)


async def fetch_release(
    link_type: str,
    resource_id: str,
    settings: Settings,
) -> NormalizedSpotifyRelease:
    cached = await get_cached_release(link_type, resource_id)
    if cached is not None:
        await persist_spotify_release(cached)
        return cached

    if link_type == "track":
        release = await fetch_track(resource_id, settings)
    else:
        release = await fetch_album(resource_id, settings)

    await set_cached_release(
        link_type,
        resource_id,
        release,
        settings.spotify_meta_cache_ttl_seconds,
    )
    await persist_spotify_release(release)
    return release

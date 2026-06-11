import asyncio
from pathlib import Path
from unittest.mock import AsyncMock

import pytest

from bot.config import Settings
from bot.services import spotify_client
from bot.services.spotify_cache import (
    get_cached_release,
    get_cached_youtube_video_id,
    meta_cache_key,
    set_cached_release,
    set_cached_youtube_video_id,
)
from bot.services.spotify_handler import _download_one_track, _send_downloaded_tracks
from bot.services.youtube_audio import YoutubeAudioError
from bot.services.spotify_models import NormalizedSpotifyRelease, NormalizedSpotifyTrack, release_to_dict
from bot.services.spotify_parser import SpotifyLink
from bot.services.youtube_audio import build_track_search_query


class FakeReplyMessage:
    def __init__(self):
        self.edited_texts: list[str] = []

    async def edit_text(self, text: str):
        self.edited_texts.append(text)


class FakeBot:
    def __init__(self):
        self.sent_audio: list[dict] = []

    async def send_audio(self, **kwargs):
        self.sent_audio.append(kwargs)


class FakeMessage:
    def __init__(self):
        self.chat = type("Chat", (), {"id": 10})()
        self.bot = FakeBot()
        self.replies: list[FakeReplyMessage] = []

    async def reply(self, text: str):
        reply = FakeReplyMessage()
        self.replies.append(reply)
        return reply


def _settings(**overrides) -> Settings:
    base = {
        "bot_token": "test-token",
        "spotify_download_concurrency": 2,
        "spotify_track_timeout_seconds": 15,
        "spotify_meta_cache_ttl_seconds": 3600,
        "youtube_search_cache_ttl_seconds": 604800,
    }
    base.update(overrides)
    return Settings(**base)


def _release(track_count: int = 3) -> NormalizedSpotifyRelease:
    tracks = [
        NormalizedSpotifyTrack(
            source_id=f"id-{index}",
            disc_number=1,
            track_number=index,
            title=f"Track {index}",
            artists="Artist",
            duration_ms=180000,
            spotify_url=f"https://open.spotify.com/track/id-{index}",
        )
        for index in range(1, track_count + 1)
    ]
    return NormalizedSpotifyRelease(
        source_id="album-id",
        title="Album",
        album_type="album",
        artists="Artist",
        release_date="2021-01-01",
        cover_url=None,
        spotify_url="https://open.spotify.com/album/album-id",
        tracks=tracks,
    )


@pytest.fixture
async def fake_redis(monkeypatch):
    import fakeredis.aioredis

    server = fakeredis.FakeServer()
    async_client = fakeredis.aioredis.FakeRedis(server=server, decode_responses=True)

    async def _async_redis():
        return async_client

    monkeypatch.setattr("bot.services.redis_client._async_redis", async_client)
    monkeypatch.setattr("bot.services.redis_client.get_async_redis", _async_redis)
    return async_client


async def test_parallel_download_respects_concurrency_limit(monkeypatch, tmp_path: Path):
    active = 0
    max_active = 0
    lock = asyncio.Lock()
    settings = _settings(spotify_download_concurrency=2)

    async def fake_download_audio(track, track_dir, settings_obj, semaphore):
        nonlocal active, max_active
        async with semaphore:
            async with lock:
                active += 1
                max_active = max(max_active, active)
            await asyncio.sleep(0.05)
            async with lock:
                active -= 1
            audio_path = track_dir / f"{track.title}.mp3"
            audio_path.write_bytes(b"audio")
            return str(audio_path)

    monkeypatch.setattr(
        "bot.services.spotify_handler._download_track_audio",
        fake_download_audio,
    )

    class FakeTempDir:
        def __enter__(self):
            return tmp_path

        def __exit__(self, *args):
            return False

    monkeypatch.setattr(
        "bot.services.spotify_handler.tempfile_manager",
        lambda _task_id: FakeTempDir(),
    )
    monkeypatch.setattr(
        "bot.services.spotify_handler._try_acquire_release_download_lock",
        AsyncMock(return_value=True),
    )
    monkeypatch.setattr(
        "bot.services.spotify_handler._release_release_download_lock",
        AsyncMock(),
    )

    message = FakeMessage()
    await _send_downloaded_tracks(
        message,
        _release(track_count=4),
        settings,
        "en",
        SpotifyLink(type="album", id="album-id"),
    )

    assert max_active <= 2
    assert len(message.bot.sent_audio) == 4


async def test_one_failed_track_does_not_cancel_others(monkeypatch, tmp_path: Path):
    settings = _settings(spotify_download_concurrency=3)

    async def fake_download_audio(track, track_dir, settings_obj, semaphore):
        async with semaphore:
            if track.track_number == 2:
                raise YoutubeAudioError("simulated failure")
            audio_path = track_dir / f"{track.title}.mp3"
            audio_path.write_bytes(b"audio")
            return str(audio_path)

    monkeypatch.setattr(
        "bot.services.spotify_handler._download_track_audio",
        fake_download_audio,
    )

    class FakeTempDir:
        def __enter__(self):
            return tmp_path

        def __exit__(self, *args):
            return False

    monkeypatch.setattr(
        "bot.services.spotify_handler.tempfile_manager",
        lambda _task_id: FakeTempDir(),
    )
    monkeypatch.setattr(
        "bot.services.spotify_handler._try_acquire_release_download_lock",
        AsyncMock(return_value=True),
    )
    monkeypatch.setattr(
        "bot.services.spotify_handler._release_release_download_lock",
        AsyncMock(),
    )

    message = FakeMessage()
    await _send_downloaded_tracks(
        message,
        _release(track_count=3),
        settings,
        "en",
        SpotifyLink(type="album", id="album-id"),
    )

    assert len(message.bot.sent_audio) == 2


async def test_spotify_metadata_cache_hit_skips_api(fake_redis, monkeypatch):
    settings = _settings()
    release = _release(track_count=1)
    await fake_redis.set(
        meta_cache_key("album", "album-id"),
        __import__("json").dumps(release_to_dict(release)),
    )

    fetch_album = AsyncMock()
    monkeypatch.setattr(spotify_client, "fetch_album", fetch_album)

    cached = await get_cached_release("album", "album-id")
    assert cached is not None
    assert cached.title == "Album"

    result = await spotify_client.fetch_release("album", "album-id", settings)
    assert result.title == "Album"
    fetch_album.assert_not_called()


async def test_spotify_metadata_cache_miss_fetches_and_stores(fake_redis, monkeypatch):
    settings = _settings()
    release = _release(track_count=1)

    monkeypatch.setattr(
        spotify_client,
        "fetch_album",
        AsyncMock(return_value=release),
    )

    result = await spotify_client.fetch_release("album", "album-id", settings)

    assert result.title == "Album"
    cached_raw = await fake_redis.get(meta_cache_key("album", "album-id"))
    assert cached_raw is not None


async def test_spotify_metadata_cache_read_failure_falls_back(monkeypatch):
    settings = _settings()
    release = _release(track_count=1)

    async def broken_redis():
        raise ConnectionError("redis down")

    monkeypatch.setattr("bot.services.spotify_cache.get_async_redis", broken_redis)
    fetch_album = AsyncMock(return_value=release)
    monkeypatch.setattr(spotify_client, "fetch_album", fetch_album)

    result = await spotify_client.fetch_release("album", "album-id", settings)

    assert result.title == "Album"
    fetch_album.assert_awaited_once()


async def test_youtube_search_cache_roundtrip(fake_redis):
    track = _release(track_count=1).tracks[0]
    query = build_track_search_query(track)

    assert await get_cached_youtube_video_id(query) is None
    await set_cached_youtube_video_id(query, "abc123", 604800)
    assert await get_cached_youtube_video_id(query) == "abc123"


async def test_download_one_track_returns_error_without_raising(monkeypatch, tmp_path: Path):
    settings = _settings()

    async def failing_download_audio(*_args, **_kwargs):
        raise YoutubeAudioError("download failed")

    monkeypatch.setattr(
        "bot.services.spotify_handler._download_track_audio",
        failing_download_audio,
    )

    track = _release(track_count=1).tracks[0]
    result = await _download_one_track(
        1,
        track,
        tmp_path,
        settings,
        asyncio.Semaphore(1),
    )

    assert result.error is not None
    assert result.audio_path is None

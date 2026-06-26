import pytest
from unittest.mock import AsyncMock

from bot.services.spotify_models import (
    NormalizedSpotifyRelease,
    NormalizedSpotifyTrack,
    normalize_album,
    normalize_track,
    release_from_track,
)
from bot.services.spotify_client import (
    SpotifyNotFoundError,
    SpotifyRateLimitError,
    _request,
    fetch_release,
)


ALBUM_FIXTURE = {
    "id": "4aawyAB9rmqOaP8fadcCl4",
    "name": "Test Album",
    "album_type": "album",
    "release_date": "2021-05-21",
    "artists": [{"name": "Artist One"}, {"name": "Artist Two"}],
    "external_urls": {"spotify": "https://open.spotify.com/album/4aawyAB9rmqOaP8fadcCl4"},
    "images": [{"url": "https://i.scdn.co/image/cover.jpg"}],
}

TRACKS_FIXTURE = [
    {
        "id": "111",
        "disc_number": 1,
        "track_number": 1,
        "name": "Track One",
        "duration_ms": 180000,
        "artists": [{"name": "Artist One"}],
        "external_urls": {"spotify": "https://open.spotify.com/track/111"},
    },
    {
        "id": "222",
        "disc_number": 1,
        "track_number": 2,
        "name": "Track Two",
        "duration_ms": 200000,
        "artists": [{"name": "Artist Two"}],
        "external_urls": {"spotify": "https://open.spotify.com/track/222"},
    },
]

TRACK_FIXTURE = {
    "id": "0VjIjW4GlUZAMYd2vXMi3b",
    "name": "Blinding Lights",
    "duration_ms": 200000,
    "disc_number": 1,
    "track_number": 1,
    "artists": [{"name": "The Weeknd"}],
    "external_urls": {"spotify": "https://open.spotify.com/track/0VjIjW4GlUZAMYd2vXMi3b"},
    "album": {
        "id": "album123456789012345678",
        "name": "After Hours",
        "album_type": "album",
        "release_date": "2020-03-20",
        "images": [{"url": "https://i.scdn.co/image/track-cover.jpg"}],
    },
}


class TestNormalizeAlbum:
    def test_normalize_album_metadata(self):
        album = normalize_album(ALBUM_FIXTURE, TRACKS_FIXTURE)

        assert isinstance(album, NormalizedSpotifyRelease)
        assert album.source_id == "4aawyAB9rmqOaP8fadcCl4"
        assert album.title == "Test Album"
        assert album.album_type == "album"
        assert album.artists == "Artist One, Artist Two"
        assert album.release_date == "2021-05-21"
        assert album.cover_url == "https://i.scdn.co/image/cover.jpg"
        assert album.spotify_url == "https://open.spotify.com/album/4aawyAB9rmqOaP8fadcCl4"
        assert len(album.tracks) == 2
        assert album.tracks[0] == NormalizedSpotifyTrack(
            source_id="111",
            track_number=1,
            title="Track One",
            artists="Artist One",
            duration_ms=180000,
            spotify_url="https://open.spotify.com/track/111",
            disc_number=1,
        )

    def test_normalize_single_album_type(self):
        single_fixture = {**ALBUM_FIXTURE, "album_type": "single", "name": "Single Release"}
        album = normalize_album(single_fixture, TRACKS_FIXTURE[:1])
        assert album.album_type == "single"
        assert len(album.tracks) == 1

    def test_normalize_empty_tracks(self):
        album = normalize_album(ALBUM_FIXTURE, [])
        assert album.tracks == []


class TestNormalizeTrack:
    def test_normalize_track_metadata(self):
        track = normalize_track(TRACK_FIXTURE)
        assert track.source_id == "0VjIjW4GlUZAMYd2vXMi3b"
        assert track.title == "Blinding Lights"
        assert track.artists == "The Weeknd"
        assert track.duration_ms == 200000

    def test_release_from_track(self):
        release = release_from_track(TRACK_FIXTURE)
        assert release.title == "Blinding Lights"
        assert release.album_type == "track"
        assert len(release.tracks) == 1
        assert release.cover_url == "https://i.scdn.co/image/track-cover.jpg"


class TestSpotifyHttpErrors:
    async def test_request_raises_404(self, monkeypatch):
        async def fake_http_json(method, url, *, headers=None, data=None, timeout=10.0):
            return 404, {}, {}

        monkeypatch.setattr("bot.services.spotify_client._http_json", fake_http_json)

        with pytest.raises(SpotifyNotFoundError):
            await _request("GET", "https://api.spotify.com/v1/albums/missing", timeout=1.0)


class TestSpotify429Retry:
    async def test_request_retries_on_429_then_succeeds(self, monkeypatch):
        calls = {"count": 0}

        async def fake_http_json(method, url, *, headers=None, data=None, timeout=10.0):
            calls["count"] += 1
            if calls["count"] == 1:
                return 429, {"retry-after": "0"}, {}
            return 200, {}, {"ok": True}

        async def fake_sleep(_seconds):
            return None

        monkeypatch.setattr("bot.services.spotify_client._http_json", fake_http_json)
        monkeypatch.setattr("bot.services.spotify_client.asyncio.sleep", fake_sleep)

        payload = await _request("GET", "https://api.spotify.com/v1/test", timeout=1.0)

        assert payload == {"ok": True}
        assert calls["count"] == 2

    async def test_request_raises_after_max_retries(self, monkeypatch):
        async def fake_http_json(method, url, *, headers=None, data=None, timeout=10.0):
            return 429, {}, {}

        async def fake_sleep(_seconds):
            return None

        monkeypatch.setattr("bot.services.spotify_client._http_json", fake_http_json)
        monkeypatch.setattr("bot.services.spotify_client.asyncio.sleep", fake_sleep)

        with pytest.raises(SpotifyRateLimitError):
            await _request("GET", "https://api.spotify.com/v1/test", timeout=1.0)


class TestFetchReleasePersistence:
    async def test_fetch_release_persists_cached_release(self, monkeypatch):
        release = normalize_album(ALBUM_FIXTURE, TRACKS_FIXTURE)
        persist_mock = AsyncMock()
        monkeypatch.setattr(
            "bot.services.spotify_client.get_cached_release",
            AsyncMock(return_value=release),
        )
        monkeypatch.setattr("bot.services.spotify_client.persist_spotify_release", persist_mock)

        from bot.config import Settings

        result = await fetch_release("album", "4aawyAB9rmqOaP8fadcCl4", Settings(bot_token="t"))

        assert result is release
        persist_mock.assert_awaited_once_with(release)

    async def test_fetch_release_persists_fresh_release(self, monkeypatch):
        release = normalize_album(ALBUM_FIXTURE, TRACKS_FIXTURE)
        persist_mock = AsyncMock()
        monkeypatch.setattr(
            "bot.services.spotify_client.get_cached_release",
            AsyncMock(return_value=None),
        )
        monkeypatch.setattr(
            "bot.services.spotify_client.fetch_album",
            AsyncMock(return_value=release),
        )
        monkeypatch.setattr(
            "bot.services.spotify_client.set_cached_release",
            AsyncMock(),
        )
        monkeypatch.setattr("bot.services.spotify_client.persist_spotify_release", persist_mock)

        from bot.config import Settings

        result = await fetch_release("album", "4aawyAB9rmqOaP8fadcCl4", Settings(bot_token="t"))

        assert result is release
        persist_mock.assert_awaited_once_with(release)

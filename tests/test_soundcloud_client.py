import pytest

from bot.config import Settings
from bot.services.runtime_settings import set_runtime_value
from bot.services.soundcloud_client import (
    SoundCloudPlaylistTooLargeError,
    _normalize_release,
    _normalize_track,
    fetch_release,
)
from bot.services.soundcloud_models import NormalizedSoundCloudRelease
from bot.services.soundcloud_parser import SoundCloudLink


async def _no_cached_release(_url):
    return None


class TestSoundCloudClient:
    def test_normalize_single_track(self):
        info = {
            "id": "12345",
            "title": "Test Track",
            "uploader": "Test Artist",
            "duration": 222.5,
            "thumbnail": "https://example.com/art.jpg",
            "webpage_url": "https://soundcloud.com/artist/test-track",
            "genre": "Electronic",
            "description": "A test track",
            "upload_date": "20240101",
        }
        release = _normalize_release(info, "https://soundcloud.com/artist/test-track")
        assert release.release_type == "track"
        assert release.title == "Test Track"
        assert release.artist == "Test Artist"
        assert len(release.tracks) == 1
        track = release.tracks[0]
        assert track.duration_ms == 222500
        assert track.genre == "Electronic"

    def test_normalize_playlist(self):
        info = {
            "id": "playlist-1",
            "title": "My Playlist",
            "uploader": "DJ Test",
            "webpage_url": "https://soundcloud.com/artist/sets/my-playlist",
            "thumbnail": "https://example.com/cover.jpg",
            "entries": [
                {
                    "id": "t1",
                    "title": "Track One",
                    "uploader": "DJ Test",
                    "duration": 180,
                    "webpage_url": "https://soundcloud.com/artist/track-one",
                },
                {
                    "id": "t2",
                    "title": "Track Two",
                    "uploader": "DJ Test",
                    "duration": 200,
                    "webpage_url": "https://soundcloud.com/artist/track-two",
                },
            ],
        }
        release = _normalize_release(info, "https://soundcloud.com/artist/sets/my-playlist")
        assert release.release_type == "playlist"
        assert release.title == "My Playlist"
        assert len(release.tracks) == 2
        assert release.tracks[0].track_number == 1
        assert release.tracks[1].track_number == 2

    def test_normalize_track_uses_tags_for_genre(self):
        track = _normalize_track({"tags": ["House", "Dance"], "title": "X"})
        assert track.genre == "House"


async def test_fetch_release_uses_runtime_soundcloud_playlist_limit(fake_redis, monkeypatch):
    settings = Settings(bot_token="test-token", soundcloud_max_tracks=20)
    await set_runtime_value("soundcloud.max_tracks_per_playlist", 100)

    monkeypatch.setattr("bot.services.soundcloud_client.get_cached_release", _no_cached_release)

    async def _set_cached_release(*_args, **_kwargs):
        return None

    monkeypatch.setattr("bot.services.soundcloud_client.set_cached_release", _set_cached_release)
    monkeypatch.setattr(
        "bot.services.soundcloud_client._extract_metadata",
        lambda _url, _settings: {
            "id": "playlist-1",
            "title": "Runtime Playlist",
            "webpage_url": "https://soundcloud.com/artist/sets/runtime",
            "entries": [
                {
                    "id": f"track-{index}",
                    "title": f"Track {index}",
                    "webpage_url": f"https://soundcloud.com/artist/track-{index}",
                }
                for index in range(1, 51)
            ],
        },
    )

    release = await fetch_release(
        SoundCloudLink(type="playlist", url="https://soundcloud.com/artist/sets/runtime"),
        settings,
    )

    assert len(release.tracks) == 50


async def test_fetch_release_reports_runtime_soundcloud_playlist_limit(fake_redis, monkeypatch):
    settings = Settings(bot_token="test-token", soundcloud_max_tracks=20)
    await set_runtime_value("soundcloud.max_tracks_per_playlist", 100)

    monkeypatch.setattr("bot.services.soundcloud_client.get_cached_release", _no_cached_release)
    monkeypatch.setattr(
        "bot.services.soundcloud_client._extract_metadata",
        lambda _url, _settings: {
            "id": "playlist-1",
            "title": "Runtime Playlist",
            "webpage_url": "https://soundcloud.com/artist/sets/runtime",
            "entries": [
                {
                    "id": f"track-{index}",
                    "title": f"Track {index}",
                    "webpage_url": f"https://soundcloud.com/artist/track-{index}",
                }
                for index in range(1, 102)
            ],
        },
    )

    with pytest.raises(SoundCloudPlaylistTooLargeError, match="limit is 100"):
        await fetch_release(
            SoundCloudLink(type="playlist", url="https://soundcloud.com/artist/sets/runtime"),
            settings,
        )

from bot.services.spotify_models import SpotifyAlbum, SpotifyTrack, normalize_album
from bot.services.spotify_client import SpotifyRateLimitError, _request


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
        "disc_number": 1,
        "track_number": 1,
        "name": "Track One",
        "duration_ms": 180000,
        "artists": [{"name": "Artist One"}],
        "external_urls": {"spotify": "https://open.spotify.com/track/111"},
    },
    {
        "disc_number": 1,
        "track_number": 2,
        "name": "Track Two",
        "duration_ms": 200000,
        "artists": [{"name": "Artist Two"}],
        "external_urls": {"spotify": "https://open.spotify.com/track/222"},
    },
]


class TestNormalizeAlbum:
    def test_normalize_album_metadata(self):
        album = normalize_album(ALBUM_FIXTURE, TRACKS_FIXTURE)

        assert isinstance(album, SpotifyAlbum)
        assert album.album_id == "4aawyAB9rmqOaP8fadcCl4"
        assert album.album_name == "Test Album"
        assert album.album_type == "album"
        assert album.artists == "Artist One, Artist Two"
        assert album.release_date == "2021-05-21"
        assert album.cover_url == "https://i.scdn.co/image/cover.jpg"
        assert album.spotify_url == "https://open.spotify.com/album/4aawyAB9rmqOaP8fadcCl4"
        assert len(album.tracks) == 2
        assert album.tracks[0] == SpotifyTrack(
            disc_number=1,
            track_number=1,
            title="Track One",
            artists="Artist One",
            duration_ms=180000,
            spotify_url="https://open.spotify.com/track/111",
        )

    def test_normalize_single_album_type(self):
        single_fixture = {**ALBUM_FIXTURE, "album_type": "single", "name": "Single Release"}
        album = normalize_album(single_fixture, TRACKS_FIXTURE[:1])
        assert album.album_type == "single"
        assert len(album.tracks) == 1


class TestSpotify429Retry:
    def test_request_retries_on_429_then_succeeds(self, monkeypatch):
        calls = {"count": 0}

        def fake_http_json(method, url, *, headers=None, data=None, timeout=10.0):
            calls["count"] += 1
            if calls["count"] == 1:
                return 429, {"retry-after": "0"}, {}
            return 200, {}, {"ok": True}

        monkeypatch.setattr("bot.services.spotify_client._http_json", fake_http_json)
        monkeypatch.setattr("bot.services.spotify_client.time.sleep", lambda _seconds: None)

        payload = _request("GET", "https://api.spotify.com/v1/test", timeout=1.0)

        assert payload == {"ok": True}
        assert calls["count"] == 2

    def test_request_raises_after_max_retries(self, monkeypatch):
        def fake_http_json(method, url, *, headers=None, data=None, timeout=10.0):
            return 429, {}, {}

        monkeypatch.setattr("bot.services.spotify_client._http_json", fake_http_json)
        monkeypatch.setattr("bot.services.spotify_client.time.sleep", lambda _seconds: None)

        try:
            _request("GET", "https://api.spotify.com/v1/test", timeout=1.0)
        except SpotifyRateLimitError:
            return

        raise AssertionError("Expected SpotifyRateLimitError")

from bot.services.soundcloud_client import _normalize_release, _normalize_track
from bot.services.soundcloud_models import NormalizedSoundCloudRelease


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

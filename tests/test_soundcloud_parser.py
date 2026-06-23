from bot.services.soundcloud_parser import SoundCloudLink, parse_soundcloud_link


class TestSoundCloudParser:
    def test_parse_track_url(self):
        link = parse_soundcloud_link("https://soundcloud.com/artist/track-name")
        assert link == SoundCloudLink(
            type="track",
            url="https://soundcloud.com/artist/track-name",
        )

    def test_parse_playlist_url(self):
        link = parse_soundcloud_link("https://soundcloud.com/artist/sets/playlist-name")
        assert link == SoundCloudLink(
            type="playlist",
            url="https://soundcloud.com/artist/sets/playlist-name",
        )

    def test_parse_short_url(self):
        link = parse_soundcloud_link("https://on.soundcloud.com/abc123")
        assert link == SoundCloudLink(type="short", url="https://on.soundcloud.com/abc123")

    def test_parse_url_with_query_params(self):
        link = parse_soundcloud_link(
            "https://soundcloud.com/artist/track-name?in=artist/sets/playlist"
        )
        assert link == SoundCloudLink(
            type="track",
            url="https://soundcloud.com/artist/track-name",
        )

    def test_parse_url_with_hash(self):
        link = parse_soundcloud_link("https://soundcloud.com/artist/sets/playlist-name#comments")
        assert link == SoundCloudLink(
            type="playlist",
            url="https://soundcloud.com/artist/sets/playlist-name",
        )

    def test_invalid_url(self):
        assert parse_soundcloud_link("https://example.com/track") is None

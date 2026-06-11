from bot.services.spotify_parser import (
    SpotifyLink,
    is_valid_spotify_id,
    parse_spotify_link,
)


class TestSpotifyParser:
    def test_parse_album_url(self):
        link = parse_spotify_link("https://open.spotify.com/album/4aawyAB9rmqOaP8fadcCl4")
        assert link == SpotifyLink(type="album", id="4aawyAB9rmqOaP8fadcCl4")

    def test_parse_track_url(self):
        link = parse_spotify_link("https://open.spotify.com/track/0VjIjW4GlUZAMYd2vXMi3b")
        assert link == SpotifyLink(type="track", id="0VjIjW4GlUZAMYd2vXMi3b")

    def test_parse_album_uri(self):
        link = parse_spotify_link("spotify:album:4aawyAB9rmqOaP8fadcCl4")
        assert link == SpotifyLink(type="album", id="4aawyAB9rmqOaP8fadcCl4")

    def test_parse_track_uri(self):
        link = parse_spotify_link("spotify:track:0VjIjW4GlUZAMYd2vXMi3b")
        assert link == SpotifyLink(type="track", id="0VjIjW4GlUZAMYd2vXMi3b")

    def test_parse_url_with_query_params(self):
        link = parse_spotify_link(
            "https://open.spotify.com/album/4aawyAB9rmqOaP8fadcCl4?si=abc123"
        )
        assert link == SpotifyLink(type="album", id="4aawyAB9rmqOaP8fadcCl4")

    def test_invalid_spotify_id(self):
        assert is_valid_spotify_id("abc") is False
        assert parse_spotify_link("https://open.spotify.com/track/abc") is None

from db.models import Platform
from bot.services.link_parser import (
    extract_urls,
    extract_spotify_link,
    extract_x_status_id,
    is_youtube_shorts,
)


class TestLinkParser:
    def test_youtube_standard(self):
        urls = extract_urls("check this https://www.youtube.com/watch?v=dQw4w9WgXcQ out")
        assert len(urls) == 1
        assert urls[0].platform == Platform.YOUTUBE
        assert "dQw4w9WgXcQ" in urls[0].url

    def test_youtube_short(self):
        urls = extract_urls("https://youtu.be/dQw4w9WgXcQ")
        assert len(urls) == 1
        assert urls[0].platform == Platform.YOUTUBE

    def test_youtube_shorts(self):
        urls = extract_urls("https://www.youtube.com/shorts/abc123def45")
        assert len(urls) == 1
        assert urls[0].platform == Platform.YOUTUBE

    def test_youtube_mobile_shorts(self):
        urls = extract_urls("https://m.youtube.com/shorts/abc123def45")
        assert len(urls) == 1
        assert urls[0].platform == Platform.YOUTUBE

    def test_is_youtube_shorts(self):
        assert is_youtube_shorts(
            "https://www.youtube.com/shorts/PuZXo75tdK8?feature=share"
        )
        assert is_youtube_shorts("https://m.youtube.com/shorts/abc123def45")
        assert not is_youtube_shorts("https://www.youtube.com/watch?v=dQw4w9WgXcQ")
        assert not is_youtube_shorts("https://youtu.be/dQw4w9WgXcQ")

    def test_tiktok(self):
        urls = extract_urls("https://www.tiktok.com/@user/video/1234567890123456789")
        assert len(urls) == 1
        assert urls[0].platform == Platform.TIKTOK

    def test_tiktok_short(self):
        urls = extract_urls("https://vm.tiktok.com/abc1234/")
        assert len(urls) == 1
        assert urls[0].platform == Platform.TIKTOK

    def test_tiktok_vt_short(self):
        urls = extract_urls("https://vt.tiktok.com/ZSxv29fme/")
        assert len(urls) == 1
        assert urls[0].platform == Platform.TIKTOK

    def test_tiktok_video_with_query(self):
        urls = extract_urls(
            "https://www.tiktok.com/@simplegamer47/video/7644167500669717781?is_from_webapp=1&sender_device=pc"
        )
        assert len(urls) == 1
        assert urls[0].platform == Platform.TIKTOK
        assert urls[0].url.endswith("sender_device=pc")

    def test_instagram_reel(self):
        urls = extract_urls("https://www.instagram.com/reel/abc123/")
        assert len(urls) == 1
        assert urls[0].platform == Platform.INSTAGRAM

    def test_instagram_reel_with_share_query(self):
        urls = extract_urls(
            "https://www.instagram.com/reel/DXyPEDrMKV1/?igsh=MTlmNzlwc2lqeGhtMA=="
        )
        assert len(urls) == 1
        assert urls[0].platform == Platform.INSTAGRAM
        assert urls[0].url.endswith("?igsh=MTlmNzlwc2lqeGhtMA==")

    def test_instagram_reels_plural(self):
        urls = extract_urls("https://www.instagram.com/reels/DXyPEDrMKV1/")
        assert len(urls) == 1
        assert urls[0].platform == Platform.INSTAGRAM

    def test_instagram_share_reel(self):
        urls = extract_urls("https://www.instagram.com/share/reel/BAEo123abc/")
        assert len(urls) == 1
        assert urls[0].platform == Platform.INSTAGRAM

    def test_instagram_instagr_am_post(self):
        urls = extract_urls("https://instagr.am/p/ABC123xyz/")
        assert len(urls) == 1
        assert urls[0].platform == Platform.INSTAGRAM

    def test_instagram_story(self):
        urls = extract_urls("https://www.instagram.com/stories/username/1234567890123456789/")
        assert len(urls) == 1
        assert urls[0].platform == Platform.INSTAGRAM

    def test_x_status(self):
        urls = extract_urls("https://x.com/user/status/1234567890123456789")
        assert len(urls) == 1
        assert urls[0].platform == Platform.X

    def test_twitter_status_with_query(self):
        urls = extract_urls("https://twitter.com/user/status/1234567890123456789?s=20")
        assert len(urls) == 1
        assert urls[0].platform == Platform.X
        assert urls[0].url.endswith("?s=20")

    def test_x_status_id_extracted_from_x_domain(self):
        """Test that x.com URLs have x_status_id populated."""
        urls = extract_urls("https://x.com/user/status/1234567890123456789")
        assert len(urls) == 1
        assert urls[0].platform == Platform.X
        assert urls[0].x_status_id == "1234567890123456789"

    def test_x_status_id_extracted_from_twitter_domain(self):
        """Test that twitter.com URLs have x_status_id populated."""
        urls = extract_urls("https://twitter.com/user/status/9876543210987654321?s=20")
        assert len(urls) == 1
        assert urls[0].platform == Platform.X
        assert urls[0].x_status_id == "9876543210987654321"
        assert urls[0].url.endswith("?s=20")

    def test_extract_x_status_id(self):
        assert extract_x_status_id("https://x.com/user/status/12345") == "12345"
        assert extract_x_status_id("https://twitter.com/user/status/67890?s=20") == "67890"
        assert extract_x_status_id("https://example.com") is None
        assert extract_x_status_id("") is None

    def test_unsupported(self):
        urls = extract_urls("https://example.com/video.mp4")
        assert len(urls) == 1
        assert urls[0].platform == Platform.UNKNOWN

    def test_multiple_urls(self):
        urls = extract_urls(
            "https://youtube.com/watch?v=abc https://tiktok.com/@x/video/123"
        )
        assert len(urls) == 2

    def test_no_urls(self):
        urls = extract_urls("just some text without links")
        assert urls == []

    def test_spotify_album_url(self):
        urls = extract_urls("https://open.spotify.com/album/4aawyAB9rmqOaP8fadcCl4")
        assert len(urls) == 1
        assert urls[0].platform == Platform.SPOTIFY
        assert urls[0].spotify_link is not None
        assert urls[0].spotify_link.type == "album"
        assert urls[0].spotify_link.id == "4aawyAB9rmqOaP8fadcCl4"

    def test_spotify_track_url(self):
        urls = extract_urls("https://open.spotify.com/track/0VjIjW4GlUZAMYd2vXMi3b")
        assert len(urls) == 1
        assert urls[0].platform == Platform.SPOTIFY
        assert urls[0].spotify_link is not None
        assert urls[0].spotify_link.type == "track"
        assert urls[0].spotify_link.id == "0VjIjW4GlUZAMYd2vXMi3b"

    def test_spotify_album_url_with_query(self):
        urls = extract_urls(
            "https://open.spotify.com/album/4aawyAB9rmqOaP8fadcCl4?si=abc123"
        )
        assert len(urls) == 1
        assert urls[0].platform == Platform.SPOTIFY
        assert urls[0].spotify_link is not None
        assert urls[0].spotify_link.id == "4aawyAB9rmqOaP8fadcCl4"
        assert "?si=abc123" in urls[0].url

    def test_spotify_album_uri(self):
        urls = extract_urls("listen spotify:album:4aawyAB9rmqOaP8fadcCl4 now")
        assert len(urls) == 1
        assert urls[0].platform == Platform.SPOTIFY
        assert urls[0].spotify_link is not None
        assert urls[0].spotify_link.id == "4aawyAB9rmqOaP8fadcCl4"
        assert urls[0].url == "spotify:album:4aawyAB9rmqOaP8fadcCl4"

    def test_spotify_track_uri(self):
        urls = extract_urls("spotify:track:0VjIjW4GlUZAMYd2vXMi3b")
        assert len(urls) == 1
        assert urls[0].spotify_link is not None
        assert urls[0].spotify_link.type == "track"

    def test_extract_spotify_link_invalid(self):
        assert extract_spotify_link("https://open.spotify.com/track/abc") is None

    def test_soundcloud_track_url(self):
        urls = extract_urls("https://soundcloud.com/artist/track-name")
        assert len(urls) == 1
        assert urls[0].platform == Platform.SOUNDCLOUD
        assert urls[0].soundcloud_link is not None
        assert urls[0].soundcloud_link.type == "track"

    def test_soundcloud_playlist_url(self):
        urls = extract_urls("https://soundcloud.com/artist/sets/playlist-name")
        assert len(urls) == 1
        assert urls[0].platform == Platform.SOUNDCLOUD
        assert urls[0].soundcloud_link is not None
        assert urls[0].soundcloud_link.type == "playlist"

    def test_soundcloud_discover_playlist_url(self):
        text = (
            "https://soundcloud.com/discover/sets/personalized-tracks::pufig:2285384033"
            "?si=baafabe621b845b4af6db0f7a39a9a1f&utm_source=clipboard"
        )
        urls = extract_urls(text)
        assert len(urls) == 1
        assert urls[0].platform == Platform.SOUNDCLOUD
        assert urls[0].soundcloud_link is not None
        assert urls[0].soundcloud_link.type == "playlist"
        assert urls[0].soundcloud_link.url.endswith("personalized-tracks::pufig:2285384033")

    def test_soundcloud_short_url(self):
        urls = extract_urls("https://on.soundcloud.com/abc123")
        assert len(urls) == 1
        assert urls[0].platform == Platform.SOUNDCLOUD
        assert urls[0].soundcloud_link is not None
        assert urls[0].soundcloud_link.type == "short"

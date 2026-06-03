from db.models import Platform
from bot.services.link_parser import extract_urls


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

    def test_tiktok(self):
        urls = extract_urls("https://www.tiktok.com/@user/video/1234567890123456789")
        assert len(urls) == 1
        assert urls[0].platform == Platform.TIKTOK

    def test_tiktok_short(self):
        urls = extract_urls("https://vm.tiktok.com/abc1234/")
        assert len(urls) == 1
        assert urls[0].platform == Platform.TIKTOK

    def test_instagram_reel(self):
        urls = extract_urls("https://www.instagram.com/reel/abc123/")
        assert len(urls) == 1
        assert urls[0].platform == Platform.INSTAGRAM

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

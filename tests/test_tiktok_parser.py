from bot.services.tiktok_parser import parse_tiktok_url, TikTokPostType


class TestTikTokParser:
    def test_parse_video_url(self):
        result = parse_tiktok_url(
            "https://www.tiktok.com/@user/video/1234567890123456789"
        )
        assert result is not None
        assert result.post_id == "1234567890123456789"
        assert result.post_type == TikTokPostType.VIDEO

    def test_parse_photo_url(self):
        result = parse_tiktok_url(
            "https://www.tiktok.com/@user/photo/1234567890123456789"
        )
        assert result is not None
        assert result.post_id == "1234567890123456789"
        assert result.post_type == TikTokPostType.PHOTO

    def test_parse_vt_short(self):
        result = parse_tiktok_url("https://vt.tiktok.com/ZSCRXPQ8F/")
        assert result is not None
        assert result.post_id == "ZSCRXPQ8F"
        assert result.post_type == TikTokPostType.UNKNOWN

    def test_parse_vm_short(self):
        result = parse_tiktok_url("https://vm.tiktok.com/abc1234/")
        assert result is not None
        assert result.post_id == "abc1234"
        assert result.post_type == TikTokPostType.UNKNOWN

    def test_parse_video_with_query(self):
        result = parse_tiktok_url(
            "https://www.tiktok.com/@user/video/1234567890123456789?is_from_webapp=1"
        )
        assert result is not None
        assert result.post_type == TikTokPostType.VIDEO

    def test_parse_photo_with_query(self):
        result = parse_tiktok_url(
            "https://www.tiktok.com/@user/photo/1234567890123456789?is_from_webapp=1"
        )
        assert result is not None
        assert result.post_type == TikTokPostType.PHOTO

    def test_parse_non_tiktok(self):
        result = parse_tiktok_url("https://www.youtube.com/watch?v=abc123")
        assert result is None

    def test_parse_malformed(self):
        result = parse_tiktok_url("https://www.tiktok.com/@user/")
        assert result is None

    def test_parse_video_in_text(self):
        result = parse_tiktok_url(
            "check this out https://www.tiktok.com/@user/video/123 abc"
        )
        assert result is not None
        assert result.post_type == TikTokPostType.VIDEO

    def test_parse_photo_in_text(self):
        result = parse_tiktok_url(
            "check this out https://www.tiktok.com/@user/photo/123 abc"
        )
        assert result is not None
        assert result.post_type == TikTokPostType.PHOTO

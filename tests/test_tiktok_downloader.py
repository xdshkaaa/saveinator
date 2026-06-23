from pathlib import Path

from workers.tiktok_downloader import (
    TikTokPostType,
    _detect_post_type,
    _guess_extension,
)


class TestTikTokDownloaderUtils:
    def test_detect_post_type_video(self, tmp_path):
        (tmp_path / "video.mp4").write_bytes(b"video")
        assert _detect_post_type(tmp_path) == TikTokPostType.VIDEO

    def test_detect_post_type_carousel(self, tmp_path):
        (tmp_path / "image_0000.jpg").write_bytes(b"img1")
        (tmp_path / "image_0001.png").write_bytes(b"img2")
        assert _detect_post_type(tmp_path) == TikTokPostType.CAROUSEL

    def test_detect_post_type_audio_only(self, tmp_path):
        (tmp_path / "audio.mp3").write_bytes(b"audio")
        assert _detect_post_type(tmp_path) == TikTokPostType.AUDIO_ONLY

    def test_detect_post_type_unknown(self, tmp_path):
        assert _detect_post_type(tmp_path) == TikTokPostType.UNKNOWN

    def test_detect_post_type_video_preferred_over_image(self, tmp_path):
        (tmp_path / "video.mp4").write_bytes(b"video")
        (tmp_path / "image.jpg").write_bytes(b"image")
        assert _detect_post_type(tmp_path) == TikTokPostType.VIDEO

    def test_guess_extension_jpg(self):
        assert _guess_extension("https://example.com/image.jpg") == ".jpg"

    def test_guess_extension_jpeg(self):
        assert _guess_extension("https://example.com/image.jpeg") == ".jpeg"

    def test_guess_extension_png(self):
        assert _guess_extension("https://example.com/image.png") == ".png"

    def test_guess_extension_webp(self):
        assert _guess_extension("https://example.com/image.webp") == ".webp"

    def test_guess_extension_no_ext(self):
        assert _guess_extension("https://example.com/image") == ".jpg"

    def test_guess_extension_unknown_ext(self):
        assert _guess_extension("https://example.com/image.gif") == ".gif"

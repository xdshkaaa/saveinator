import io
from pathlib import Path

from workers.tiktok_downloader import (
    TikTokPostType,
    _detect_post_type,
    _download_image,
    _extract_metadata,
    _guess_extension,
    _normalize_tiktok_title,
    download_tiktok,
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


class TestNormalizeTiktokTitle:
    def test_fallback_title_without_description(self):
        assert _normalize_tiktok_title(
            "TikTok video #7650199641878842638", ""
        ) == ""

    def test_fallback_title_case_insensitive(self):
        assert _normalize_tiktok_title("tiktok video #123", "") == ""

    def test_description_overrides_fallback_title(self):
        assert _normalize_tiktok_title("TikTok video #123", "мой текст") == "мой текст"

    def test_real_title_without_description(self):
        assert _normalize_tiktok_title("нормальный title", "") == "нормальный title"

    def test_empty_title_and_description(self):
        assert _normalize_tiktok_title("", "") == ""


class TestExtractMetadata:
    def test_strips_ytdlp_fallback_title(self):
        title, author, description = _extract_metadata(
            {
                "title": "TikTok video #7650199641878842638",
                "description": "",
                "uploader": "user",
            }
        )
        assert title == ""
        assert description == ""
        assert author == "user"

    def test_prefers_description_for_title(self):
        title, _, description = _extract_metadata(
            {
                "title": "TikTok video #123",
                "description": "caption text",
            }
        )
        assert title == "caption text"
        assert description == "caption text"


def test_download_tiktok_carousel_max_images(monkeypatch, tmp_path):
    info = {
        "is_slideshow": True,
        "entries": [
            {"thumbnail": "https://example.com/one.jpg"},
            {"thumbnail": "https://example.com/two.jpg"},
            {"thumbnail": "https://example.com/three.jpg"},
        ],
    }
    downloaded: list[str] = []

    def fake_download_image(url, output_path):
        downloaded.append(url)
        output_path.write_bytes(b"image")
        return True

    monkeypatch.setattr(
        "workers.tiktok_downloader._resolve_url",
        lambda url: (url, info),
    )
    monkeypatch.setattr(
        "workers.tiktok_downloader._download_image",
        fake_download_image,
    )
    monkeypatch.setattr(
        "workers.tiktok_downloader._download_audio_from_entry",
        lambda *args, **kwargs: None,
    )

    result = download_tiktok(
        "https://www.tiktok.com/@user/photo/123",
        tmp_path,
        max_images=2,
        audio_enabled=False,
    )

    assert downloaded == [
        "https://example.com/one.jpg",
        "https://example.com/two.jpg",
    ]
    assert len(result.images) == 2
    assert result.audio is None
    assert result.post_type == TikTokPostType.CAROUSEL


def test_download_tiktok_carousel_audio_disabled_skips_audio(monkeypatch, tmp_path):
    info = {
        "is_slideshow": True,
        "entries": [{"thumbnail": "https://example.com/one.jpg"}],
    }

    def fake_download_image(url, output_path):
        output_path.write_bytes(b"image")
        return True

    def fail_audio_download(*args, **kwargs):
        raise AssertionError("audio download should be skipped")

    monkeypatch.setattr(
        "workers.tiktok_downloader._resolve_url",
        lambda url: (url, info),
    )
    monkeypatch.setattr(
        "workers.tiktok_downloader._download_image",
        fake_download_image,
    )
    monkeypatch.setattr(
        "workers.tiktok_downloader._download_audio_from_entry",
        fail_audio_download,
    )

    result = download_tiktok(
        "https://www.tiktok.com/@user/photo/123",
        tmp_path,
        audio_enabled=False,
    )

    assert len(result.images) == 1
    assert result.audio is None
    assert result.post_type == TikTokPostType.CAROUSEL


def test_download_image_uses_request_timeout(monkeypatch, tmp_path):
    captured: dict[str, int] = {}

    class FakeResponse(io.BytesIO):
        def __enter__(self):
            return self

        def __exit__(self, exc_type, exc, tb):
            return False

    def fake_urlopen(url, timeout):
        captured["timeout"] = timeout
        return FakeResponse(b"image-bytes")

    monkeypatch.setattr("workers.tiktok_downloader.urllib.request.urlopen", fake_urlopen)

    output_path = tmp_path / "image.jpg"
    assert _download_image("https://example.com/image.jpg", output_path, timeout=3)
    assert captured["timeout"] == 3
    assert output_path.read_bytes() == b"image-bytes"

from datetime import UTC, datetime
from pathlib import Path
from unittest.mock import MagicMock

import pytest

from bot.services.pinterest_models import PinterestUrlType
from workers.pinterest_downloader import (
    PinterestDownloadError,
    PinterestNoMediaError,
    download_pinterest,
)


def _make_media(
    *,
    media_id: int = 1,
    local_name: str = "image.jpg",
    alt: str = "A pin",
    video: bool = False,
):
    item = MagicMock()
    item.id = media_id
    item.src = "https://i.pinimg.com/originals/ab/cd.jpg"
    item.alt = alt
    item.origin = "https://www.pinterest.com/pin/123/"
    item.resolution = (1080, 1920)
    item.video_stream = MagicMock(url="https://v.pinimg.com/video.mp4") if video else None
    item.local_path = None

    def set_local_path(path):
        item.local_path = Path(path)

    item.set_local_path = set_local_path
    return item


class TestDownloadPinterest:
    def test_invalid_url_raises(self, tmp_path):
        with pytest.raises(PinterestDownloadError, match="Invalid"):
            download_pinterest("https://example.com/pin/1", tmp_path)

    def test_no_media_raises(self, tmp_path, monkeypatch):
        fake_dl = MagicMock()
        fake_dl.scrape_and_download.return_value = []
        monkeypatch.setattr(
            "workers.pinterest_downloader._create_client",
            lambda: fake_dl,
        )

        with pytest.raises(PinterestNoMediaError):
            download_pinterest("https://www.pinterest.com/pin/123/", tmp_path)

    def test_downloads_image_with_metadata(self, tmp_path, monkeypatch):
        fake_dl = MagicMock()
        media = _make_media(local_name="photo.jpg", alt="Sunset")
        fake_dl.scrape_and_download.return_value = [media]
        monkeypatch.setattr(
            "workers.pinterest_downloader._create_client",
            lambda: fake_dl,
        )

        def fake_scrape_and_download(url, output_dir, num, download_streams, caption):
            file_path = Path(output_dir) / "photo.jpg"
            file_path.write_bytes(b"image-bytes")
            media.set_local_path(file_path)
            return [media]

        fake_dl.scrape_and_download.side_effect = fake_scrape_and_download

        result = download_pinterest(
            "https://www.pinterest.com/pin/123/",
            tmp_path,
            max_items=5,
        )

        assert result.url_type == PinterestUrlType.PIN
        assert len(result.items) == 1
        item = result.items[0]
        assert item.media_type == "image"
        assert item.title == "Sunset"
        assert item.original_media_url == media.src
        assert item.file_size == len(b"image-bytes")

    def test_filters_videos_when_disabled(self, tmp_path, monkeypatch):
        fake_dl = MagicMock()
        image = _make_media(local_name="photo.jpg")
        video = _make_media(media_id=2, local_name="clip.mp4", video=True)

        def fake_scrape_and_download(url, output_dir, num, download_streams, caption):
            image_path = Path(output_dir) / "photo.jpg"
            image_path.write_bytes(b"img")
            image.set_local_path(image_path)
            video_path = Path(output_dir) / "clip.mp4"
            video_path.write_bytes(b"vid")
            video.set_local_path(video_path)
            return [image, video]

        fake_dl.scrape_and_download.side_effect = fake_scrape_and_download
        monkeypatch.setattr(
            "workers.pinterest_downloader._create_client",
            lambda: fake_dl,
        )

        result = download_pinterest(
            "https://www.pinterest.com/pin/123/",
            tmp_path,
            download_images=True,
            download_videos=False,
        )

        assert len(result.items) == 1
        assert result.items[0].media_type == "image"

    def test_private_error_message(self, tmp_path, monkeypatch):
        fake_dl = MagicMock()
        fake_dl.scrape_and_download.side_effect = RuntimeError("403 Forbidden private pin")
        monkeypatch.setattr(
            "workers.pinterest_downloader._create_client",
            lambda: fake_dl,
        )

        with pytest.raises(PinterestDownloadError, match="private"):
            download_pinterest("https://www.pinterest.com/pin/999/", tmp_path)

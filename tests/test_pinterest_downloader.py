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

    def test_pin_uses_pin_fetcher_not_related_pins(self, tmp_path, monkeypatch):
        media = _make_media(local_name="main.png", alt="date w Kuriyama Mirai")

        def fake_fetch_pin_media(url, dl_client, timeout):
            file_path = tmp_path / "main.png"
            file_path.write_bytes(b"main-image")
            media.set_local_path(file_path)
            return [media]

        monkeypatch.setattr(
            "workers.pinterest_downloader.fetch_pin_media",
            fake_fetch_pin_media,
        )
        monkeypatch.setattr(
            "workers.pinterest_downloader._create_client",
            lambda: MagicMock(),
        )
        monkeypatch.setattr(
            "workers.pinterest_downloader.operations.download_media",
            lambda scraped, output_dir, include_videos: scraped,
        )

        result = download_pinterest("https://www.pinterest.com/pin/607845280985287827/", tmp_path)

        assert len(result.items) == 1
        assert result.items[0].title == "date w Kuriyama Mirai"
        assert result.items[0].media_type == "image"

    def test_no_media_raises(self, tmp_path, monkeypatch):
        monkeypatch.setattr(
            "workers.pinterest_downloader.fetch_pin_media",
            lambda *args, **kwargs: (_ for _ in ()).throw(PinterestNoMediaError("empty")),
        )
        monkeypatch.setattr(
            "workers.pinterest_downloader._create_client",
            lambda: MagicMock(),
        )

        with pytest.raises(PinterestNoMediaError):
            download_pinterest("https://www.pinterest.com/pin/123/", tmp_path)

    def test_board_still_uses_scrape_and_download(self, tmp_path, monkeypatch):
        fake_dl = MagicMock()
        media = _make_media(local_name="board.jpg")

        def fake_scrape_and_download(url, output_dir, num, download_streams, caption):
            file_path = Path(output_dir) / "board.jpg"
            file_path.write_bytes(b"board")
            media.set_local_path(file_path)
            return [media]

        fake_dl.scrape_and_download.side_effect = fake_scrape_and_download
        monkeypatch.setattr(
            "workers.pinterest_downloader._create_client",
            lambda: fake_dl,
        )

        result = download_pinterest(
            "https://www.pinterest.com/user/my-board/",
            tmp_path,
            max_items=3,
        )

        assert len(result.items) == 1
        fake_dl.scrape_and_download.assert_called_once()

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
            "https://www.pinterest.com/user/my-board/",
            tmp_path,
            download_images=True,
            download_videos=False,
        )

        assert len(result.items) == 1
        assert result.items[0].media_type == "image"

    def test_private_error_message(self, tmp_path, monkeypatch):
        def fake_fetch_pin_media(*args, **kwargs):
            raise RuntimeError("403 Forbidden private pin")

        monkeypatch.setattr(
            "workers.pinterest_downloader.fetch_pin_media",
            fake_fetch_pin_media,
        )
        monkeypatch.setattr(
            "workers.pinterest_downloader._create_client",
            lambda: MagicMock(),
        )

        with pytest.raises(PinterestDownloadError, match="private"):
            download_pinterest("https://www.pinterest.com/pin/999/", tmp_path)

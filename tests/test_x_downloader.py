"""Tests for X/Twitter photo downloader."""

import json
from pathlib import Path
from unittest.mock import MagicMock

import pytest

from workers.x_downloader import (
    XPhotosNotFoundError,
    download_x_photos,
    extract_status_id,
    fetch_x_photo_urls,
)


class TestExtractStatusId:
    def test_from_x_url(self):
        assert extract_status_id(
            "https://x.com/kani_shio_l275v/status/2069807710292635758?s=46"
        ) == "2069807710292635758"


class TestFetchXPhotoUrls:
    def test_fxtwitter_response(self, monkeypatch):
        payload = {
            "code": 200,
            "tweet": {
                "text": "hello",
                "media": {
                    "photos": [
                        {"url": "https://pbs.twimg.com/media/a.jpg?name=orig"},
                        {"url": "https://pbs.twimg.com/media/b.jpg?name=orig"},
                    ]
                },
            },
        }

        class FakeResponse:
            def raise_for_status(self):
                return None

            def json(self):
                return payload

        class FakeClient:
            def __init__(self, *args, **kwargs):
                pass

            def __enter__(self):
                return self

            def __exit__(self, *args):
                return False

            def get(self, url):
                assert url.endswith("/2069807710292635758")
                return FakeResponse()

        monkeypatch.setattr("workers.x_downloader.httpx.Client", FakeClient)

        title, urls = fetch_x_photo_urls("2069807710292635758")
        assert title == "hello"
        assert len(urls) == 2

    def test_raises_when_no_photos(self, monkeypatch):
        class FakeResponse:
            def raise_for_status(self):
                return None

            def json(self):
                return {"code": 200, "tweet": {"text": "text only", "media": {"photos": []}}}

        class FakeClient:
            def __init__(self, *args, **kwargs):
                pass

            def __enter__(self):
                return self

            def __exit__(self, *args):
                return False

            def get(self, url):
                return FakeResponse()

        monkeypatch.setattr("workers.x_downloader.httpx.Client", FakeClient)

        with pytest.raises(XPhotosNotFoundError):
            fetch_x_photo_urls("123")


class TestDownloadXPhotos:
    def test_downloads_images_to_output_dir(self, monkeypatch, tmp_path: Path):
        monkeypatch.setattr(
            "workers.x_downloader.fetch_x_photo_urls",
            lambda _status_id: ("album", ["https://pbs.twimg.com/media/a.jpg?name=orig"]),
        )
        monkeypatch.setattr(
            "workers.x_downloader.get_runtime_int",
            lambda _key: 4,
        )

        def fake_download(url: str, output_path: Path) -> bool:
            output_path.write_bytes(b"jpg")
            return True

        monkeypatch.setattr("workers.x_downloader._download_image", fake_download)

        info = download_x_photos(
            "https://x.com/user/status/2069807710292635758",
            tmp_path,
            status_id="2069807710292635758",
        )

        assert info["title"] == "album"
        assert list(tmp_path.glob("photo_*.jpg")) == [tmp_path / "photo_1.jpg"]

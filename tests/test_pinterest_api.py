from pathlib import Path

import pytest
from aiohttp import web
from aiohttp.test_utils import TestClient, TestServer

from bot.api.pinterest import register_pinterest_routes
from bot.config import settings
from bot.services.pinterest_models import PinterestDownloadResult, PinterestMediaItem, PinterestUrlType


def _sample_result(url: str) -> PinterestDownloadResult:
    return PinterestDownloadResult(
        url=url,
        url_type=PinterestUrlType.PIN,
        items=[
            PinterestMediaItem(
                source_url=url,
                media_type="image",
                title="Test",
                description="Test",
                original_media_url="https://i.pinimg.com/test.jpg",
                file_path="/tmp/ytbot/task/photo.jpg",
                file_size=12,
            )
        ],
    )


class _fake_temp:
    def __init__(self, path: Path):
        self.path = path

    def __enter__(self):
        self.path.mkdir(parents=True, exist_ok=True)
        return self.path

    def __exit__(self, exc_type, exc, tb):
        return False


@pytest.fixture
def api_app(monkeypatch):
    monkeypatch.setattr(settings, "pinterest_enabled", True)
    monkeypatch.setattr(settings, "download_api_enabled", True)
    monkeypatch.setattr(settings, "pinterest_timeout_seconds", 5)
    app = web.Application()
    register_pinterest_routes(app)
    return app


@pytest.mark.asyncio
async def test_download_pinterest_api_success(api_app, monkeypatch, tmp_path):
    def fake_download(url, output_dir, max_items, download_images, download_videos):
        return _sample_result(url)

    monkeypatch.setattr("bot.api.pinterest.download_pinterest", fake_download)
    monkeypatch.setattr(
        "bot.api.pinterest.tempfile_manager",
        lambda task_id, keep_on_success=False: _fake_temp(tmp_path / task_id),
    )

    async with TestClient(TestServer(api_app)) as client:
        response = await client.post(
            "/download/pinterest",
            json={
                "url": "https://www.pinterest.com/pin/123/",
                "limit": 3,
                "downloadVideos": True,
                "downloadImages": True,
            },
        )
        assert response.status == 200
        payload = await response.json()
        assert payload["count"] == 1
        assert payload["items"][0]["media_type"] == "image"


@pytest.mark.asyncio
async def test_download_pinterest_api_invalid_url(api_app):
    async with TestClient(TestServer(api_app)) as client:
        response = await client.post(
            "/download/pinterest",
            json={"url": "https://example.com/not-pinterest"},
        )

    assert response.status == 400


@pytest.mark.asyncio
async def test_download_pinterest_api_disabled(monkeypatch):
    monkeypatch.setattr(settings, "pinterest_enabled", False)
    app = web.Application()
    register_pinterest_routes(app)

    async with TestClient(TestServer(app)) as client:
        response = await client.post(
            "/download/pinterest",
            json={"url": "https://www.pinterest.com/pin/123/"},
        )

    assert response.status == 503

"""Integration tests for X photo fallback in tasks."""

from pathlib import Path
from unittest.mock import AsyncMock

import pytest
import yt_dlp

from workers.tasks import _download_platform_media


def test_x_photo_fallback_on_no_video_error(monkeypatch, tmp_path: Path):
    def fake_ytdlp_fail(*_args, **_kwargs):
        raise yt_dlp.utils.DownloadError(
            "ERROR: [twitter] 2069807710292635758: No video could be found in this tweet"
        )

    monkeypatch.setattr("workers.tasks.download_with_reply_filter", fake_ytdlp_fail)
    monkeypatch.setattr(
        "workers.tasks.download_x_photos",
        lambda url, output_dir, status_id=None: {
            "title": "photos",
            "id": status_id or "2069807710292635758",
        },
    )

    info = _download_platform_media(
        "https://x.com/user/status/2069807710292635758",
        tmp_path,
        "best",
        "x",
        x_status_id="2069807710292635758",
    )

    assert info["title"] == "photos"


def test_non_x_platform_does_not_fallback(monkeypatch, tmp_path: Path):
    monkeypatch.setattr(
        "workers.tasks.download",
        lambda *_args, **_kwargs: (_ for _ in ()).throw(
            yt_dlp.utils.DownloadError("No video could be found in this tweet")
        ),
    )

    with pytest.raises(yt_dlp.utils.DownloadError):
        _download_platform_media(
            "https://example.com/video",
            tmp_path,
            "best",
            "youtube",
        )

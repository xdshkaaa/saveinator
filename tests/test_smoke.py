"""Smoke tests for critical service-lifecycle and timeout paths.

Covers:
- Config model loads without crashing.
- Dispatcher creates without errors.
- Celery app initialises and registers signals.
- _run_download_and_send core (success, timeout, file-too-large).
- _run_pinterest_download core (success, timeout, error).
- _edit_message / _delete_message error-logging helpers.
"""

import asyncio
import logging
from pathlib import Path
from unittest.mock import AsyncMock

import pytest

from bot.config import settings
from bot.dispatcher import create_dispatcher
from db.models import DownloadStatus
from workers.app import app as celery_app


# ── 1. Config & bootstrap ──────────────────────────────────────────────

class TestConfigAndBootstrap:
    def test_settings_loads(self):
        """Config model instantiates and exposes required fields."""
        assert settings.bot_token
        assert settings.database_url
        assert settings.redis_url
        # admin_telegram_id default is 0 (not hardcoded)
        assert settings.admin_telegram_id == 0

    def test_create_dispatcher(self):
        """Dispatcher tree boots without errors."""
        dp = create_dispatcher()
        assert dp is not None
        # Should have routers registered
        assert len(dp.sub_routers) > 0


# ── 2. Celery worker bootstrap ─────────────────────────────────────────

class TestCeleryWorker:
    def test_celery_app_name(self):
        assert celery_app.main == "saveinator"

    def test_celery_app_includes_tasks(self):
        assert "workers.tasks" in celery_app.conf.include
        assert "workers.pinterest_task" in celery_app.conf.include

    def test_celery_beat_schedule_has_cleanup(self):
        assert "sweep-tempfiles" in celery_app.conf.beat_schedule
        task = celery_app.conf.beat_schedule["sweep-tempfiles"]
        assert task["task"] == "workers.tasks.cleanup_stale_task"

    def test_celery_beat_schedule_has_tiktok_cookie_refresh(self):
        assert "tiktok-refresh-cookies" in celery_app.conf.beat_schedule
        task = celery_app.conf.beat_schedule["tiktok-refresh-cookies"]
        assert task["task"] == "workers.tiktok_task.tiktok_refresh_cookies_task"
        assert task["schedule"] == 300.0


# ── 3. _run_download_and_send (async core) ─────────────────────────────

@pytest.mark.asyncio
async def test_run_download_and_send_success(monkeypatch):
    from workers.tasks import _run_download_and_send

    bot = AsyncMock()

    def fake_download(url, output_dir, format_id):
        (output_dir / "clip.mp4").write_bytes(b"x" * 1024)
        return {"title": "ok"}

    monkeypatch.setattr("workers.tasks.download", fake_download)
    monkeypatch.setattr("workers.tasks._platform_download_timeout_seconds", lambda _p: 0)
    monkeypatch.setattr("workers.tasks._platform_max_file_mb", lambda _p: 500)
    monkeypatch.setattr("workers.tasks.release_user_lock_sync", lambda *a, **kw: None)

    await _run_download_and_send(
        bot=bot,
        task_id="test-ok",
        url="https://example.com/video",
        platform="tiktok",
        chat_id=1,
        user_id=2,
        message_id=3,
        lang="en",
        resolved_format_id="best",
        ytdlp_format="best",
        lock_token="tok",
        quality=None,
        aspect_ratio=None,
    )

    # Bot should have called edit_message (downloading), delete_message (success)
    assert bot.edit_message_text.called
    assert bot.delete_message.called


@pytest.mark.asyncio
async def test_run_download_and_send_timeout(monkeypatch):
    from workers.tasks import _run_download_and_send

    bot = AsyncMock()

    async def _timeout(*a, **kw):
        raise asyncio.TimeoutError

    monkeypatch.setattr("workers.tasks._download_with_timeout", _timeout)
    monkeypatch.setattr("workers.tasks.release_user_lock_sync", lambda *a, **kw: None)

    await _run_download_and_send(
        bot=bot,
        task_id="test-timeout",
        url="https://example.com/slow",
        platform="tiktok",
        chat_id=1,
        user_id=2,
        message_id=3,
        lang="en",
        resolved_format_id="best",
        ytdlp_format="best",
        lock_token="tok",
        quality=None,
        aspect_ratio=None,
    )

    # Timeout should result in an edit_message with the timeout text.
    assert bot.edit_message_text.called
    text = bot.edit_message_text.call_args[1].get("text", "")
    assert any(word in text.lower() for word in ("try again", "timeout", "later"))


@pytest.mark.asyncio
async def test_run_download_and_send_file_too_large(monkeypatch):
    from workers.tasks import _run_download_and_send

    bot = AsyncMock()

    def fake_download(url, output_dir, format_id):
        (output_dir / "big.mp4").write_bytes(b"x" * (60 * 1024 * 1024))
        return {"title": "big"}

    monkeypatch.setattr("workers.tasks.download", fake_download)
    monkeypatch.setattr("workers.tasks._platform_download_timeout_seconds", lambda _p: 0)
    monkeypatch.setattr("workers.tasks._platform_max_file_mb", lambda _p: 50)
    monkeypatch.setattr("workers.tasks.release_user_lock_sync", lambda *a, **kw: None)

    await _run_download_and_send(
        bot=bot,
        task_id="test-large",
        url="https://example.com/big",
        platform="tiktok",
        chat_id=1,
        user_id=2,
        message_id=3,
        lang="en",
        resolved_format_id="best",
        ytdlp_format="best",
        lock_token="tok",
        quality=None,
        aspect_ratio=None,
    )

    assert bot.edit_message_text.called
    text = bot.edit_message_text.call_args[1].get("text", "")
    assert "can't send" in text.lower() or "too large" in text.lower()


# ── 4. _run_pinterest_download (async core) ────────────────────────────

@pytest.mark.asyncio
async def test_run_pinterest_download_success(monkeypatch):
    from workers.pinterest_task import _run_pinterest_download
    from bot.services.pinterest_models import (
        PinterestDownloadResult,
        PinterestMediaItem,
        PinterestUrlType,
    )

    bot = AsyncMock()

    def fake_pin_dl(url, output_dir, max_items):
        f = output_dir / "pin.jpg"
        f.write_bytes(b"image")
        return PinterestDownloadResult(
            url=url,
            url_type=PinterestUrlType.PIN,
            items=[
                PinterestMediaItem(
                    source_url=url,
                    media_type="image",
                    title="test",
                    description=None,
                    original_media_url="https://i.pinimg.com/test.jpg",
                    file_path=str(f),
                    file_size=f.stat().st_size,
                )
            ],
        )

    monkeypatch.setattr("workers.pinterest_task.download_pinterest", fake_pin_dl)
    monkeypatch.setattr("workers.pinterest_task.send_file", AsyncMock())
    monkeypatch.setattr("workers.pinterest_task.release_user_lock_sync", lambda *a, **kw: None)
    monkeypatch.setattr("workers.pinterest_task.pinterest_timeout_seconds", lambda: 0)
    monkeypatch.setattr("workers.pinterest_task.pinterest_max_file_mb", lambda: 500)
    monkeypatch.setattr("workers.pinterest_task.send_document_limit_mb", lambda: 1999)

    await _run_pinterest_download(
        bot=bot,
        task_id="pin-ok",
        url="https://www.pinterest.com/pin/123/",
        chat_id=10,
        user_id=20,
        message_id=30,
        lang="en",
        lock_token="pin-tok",
    )

    assert bot.delete_message.called


@pytest.mark.asyncio
async def test_run_pinterest_download_timeout(monkeypatch):
    from workers.pinterest_task import _run_pinterest_download

    bot = AsyncMock()

    async def _timeout(*a, **kw):
        raise asyncio.TimeoutError

    monkeypatch.setattr("workers.pinterest_task._download_pinterest_with_timeout", _timeout)
    monkeypatch.setattr("workers.pinterest_task.release_user_lock_sync", lambda *a, **kw: None)

    await _run_pinterest_download(
        bot=bot,
        task_id="pin-timeout",
        url="https://www.pinterest.com/pin/999/",
        chat_id=10,
        user_id=20,
        message_id=30,
        lang="en",
        lock_token="pin-tok",
    )

    assert bot.edit_message_text.called
    text = bot.edit_message_text.call_args[1].get("text", "")
    assert any(word in text.lower() for word in ("try again", "timeout", "later"))


@pytest.mark.asyncio
async def test_run_pinterest_download_no_media(monkeypatch):
    from workers.pinterest_task import _run_pinterest_download
    from workers.pinterest_downloader import PinterestNoMediaError

    bot = AsyncMock()

    def fake_pin_dl(url, output_dir, max_items):
        raise PinterestNoMediaError("no media")

    monkeypatch.setattr("workers.pinterest_task.download_pinterest", fake_pin_dl)
    monkeypatch.setattr("workers.pinterest_task.send_file", AsyncMock())
    monkeypatch.setattr("workers.pinterest_task.release_user_lock_sync", lambda *a, **kw: None)

    await _run_pinterest_download(
        bot=bot,
        task_id="pin-nomedia",
        url="https://www.pinterest.com/pin/000/",
        chat_id=10,
        user_id=20,
        message_id=30,
        lang="en",
        lock_token="pin-tok",
    )

    assert bot.edit_message_text.called
    text = bot.edit_message_text.call_args[1].get("text", "")
    assert "media" in text.lower()


# ── 5. _edit_message / _delete_message error logging ───────────────────
# structlog wraps stdlib logging; caplog captures stdlib records after
# propagation.  We capture by checking the structlog output directly.

@pytest.mark.asyncio
async def test_edit_message_logs_on_failure(capsys):
    """_edit_message logs warning with chat_id, message_id, action on failure."""
    from workers.tasks import _edit_message

    bot = AsyncMock()
    bot.edit_message_text.side_effect = RuntimeError("telegram-api-error")

    await _edit_message(bot, chat_id=1, message_id=2, text="hello")

    captured = capsys.readouterr()
    assert "edit_message failed" in captured.out
    assert "chat_id=1" in captured.out
    assert "message_id=2" in captured.out
    assert "action=edit_message" in captured.out


@pytest.mark.asyncio
async def test_delete_message_logs_on_failure(capsys):
    """_delete_message logs warning with chat_id, message_id, action on failure."""
    from workers.tasks import _delete_message

    bot = AsyncMock()
    bot.delete_message.side_effect = RuntimeError("telegram-api-error")

    await _delete_message(bot, chat_id=10, message_id=20)

    captured = capsys.readouterr()
    assert "delete_message failed" in captured.out
    assert "chat_id=10" in captured.out
    assert "message_id=20" in captured.out
    assert "action=delete_message" in captured.out

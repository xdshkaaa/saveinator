from pathlib import Path

import pytest

from db.models import DownloadStatus
from workers.tasks import download_and_send_task


class FakeBot:
    def __init__(self):
        self.deleted: list[tuple[int, int]] = []
        self.edited: list[tuple[int, int, str]] = []

    async def edit_message_text(self, chat_id: int, message_id: int, text: str):
        self.edited.append((chat_id, message_id, text))

    async def delete_message(self, chat_id: int, message_id: int):
        self.deleted.append((chat_id, message_id))


def test_download_task_send_file_too_large_keeps_status_message(monkeypatch):
    fake_bot = FakeBot()
    recorded: list[tuple] = []

    def fake_download(url: str, output_dir: Path, format_id: str):
        (output_dir / "video.mp4").write_bytes(b"video")
        return {"title": "blocked"}

    async def fake_send_file(*_args, **_kwargs):
        return "too_large"

    async def fake_record_download_safe(url, platform, format_id, size_mb, status, *_args, **_kwargs):
        recorded.append((url, platform, status))

    monkeypatch.setattr("workers.tasks._get_bot", lambda: fake_bot)
    monkeypatch.setattr("workers.tasks.download", fake_download)
    monkeypatch.setattr("workers.tasks.send_file", fake_send_file)
    monkeypatch.setattr("workers.tasks.send_document_limit_mb", lambda: 1999)
    monkeypatch.setattr("workers.tasks._platform_max_file_mb", lambda _platform: 1999)
    monkeypatch.setattr("workers.tasks._record_download_safe", fake_record_download_safe)
    monkeypatch.setattr("workers.tasks._platform_download_timeout_seconds", lambda _platform: 0)
    monkeypatch.setattr("workers.tasks.release_user_lock_sync", lambda *_args, **_kwargs: None)

    download_and_send_task.run(
        url="https://www.youtube.com/watch?v=dQw4w9WgXcQ",
        format_id="best",
        platform="youtube",
        chat_id=1,
        user_id=2,
        message_id=3,
        lang="en",
    )

    assert fake_bot.deleted == []
    assert "can't send" in fake_bot.edited[-1][2]
    assert recorded[0][2] == DownloadStatus.FAILED


def test_download_task_send_file_exception_shows_error(monkeypatch):
    fake_bot = FakeBot()
    recorded: list[tuple] = []

    def fake_download(url: str, output_dir: Path, format_id: str):
        (output_dir / "video.mp4").write_bytes(b"video")
        return {"title": "broken-send"}

    async def fake_send_file(*_args, **_kwargs):
        raise RuntimeError("telegram upload failed")

    async def fake_record_download(url, platform, format_id, size_mb, status, *_args, **_kwargs):
        recorded.append((status,))

    monkeypatch.setattr("workers.tasks._get_bot", lambda: fake_bot)
    monkeypatch.setattr("workers.tasks.download", fake_download)
    monkeypatch.setattr("workers.tasks.send_file", fake_send_file)
    monkeypatch.setattr("workers.tasks._record_download", fake_record_download)
    monkeypatch.setattr("workers.tasks._platform_download_timeout_seconds", lambda _platform: 0)
    monkeypatch.setattr("workers.tasks._platform_max_file_mb", lambda _platform: 50)
    monkeypatch.setattr("workers.tasks.release_user_lock_sync", lambda *_args, **_kwargs: None)

    download_and_send_task.run(
        url="https://vt.tiktok.com/ZSxv29fme/",
        format_id="best",
        platform="tiktok",
        chat_id=1,
        user_id=2,
        message_id=3,
        lang="en",
    )

    assert fake_bot.deleted == []
    assert fake_bot.edited
    assert recorded[0][0] == DownloadStatus.FAILED


@pytest.mark.asyncio
async def test_send_file_falls_back_to_document_after_video_failure(monkeypatch, tmp_path: Path):
    from bot.services import file_sender

    video = tmp_path / "clip.mp4"
    video.write_bytes(b"x" * 1024)

    sent: list[str] = []

    class StubBot:
        async def send_video(self, **_kwargs):
            raise RuntimeError("bad codec")

        async def send_document(self, **_kwargs):
            sent.append("document")

    monkeypatch.setattr(file_sender.settings, "send_video_limit_mb", 50)
    monkeypatch.setattr(file_sender, "send_document_limit_mb", lambda: 1999)

    result = await file_sender.send_file(StubBot(), video, 1)

    assert result == "document"
    assert sent == ["document"]

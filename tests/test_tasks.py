import asyncio
from pathlib import Path

from db.models import DownloadStatus
from workers.tasks import download_and_send_task


class FakeBot:
    def __init__(self):
        self.deleted: list[tuple[int, int]] = []
        self.edited: list[tuple[int, int, str]] = []
        self.documents: list[tuple] = []

    async def edit_message_text(self, chat_id: int, message_id: int, text: str):
        self.edited.append((chat_id, message_id, text))

    async def delete_message(self, chat_id: int, message_id: int):
        self.deleted.append((chat_id, message_id))

    async def send_document(self, *args, **kwargs):
        self.documents.append((args, kwargs))


def test_download_task_accepts_direct_url_without_format_cache(monkeypatch):
    fake_bot = FakeBot()
    sent_files: list[tuple[Path, int, str]] = []
    recorded: list[tuple[str, str, str, float]] = []

    def fake_download(url: str, output_dir: Path, format_id: str):
        assert url == "https://vt.tiktok.com/ZSxv29fme/"
        assert format_id == "best"
        (output_dir / "video.mp4").write_bytes(b"video")
        return {"title": "direct"}

    async def fake_send_file(bot, path: Path, chat_id: int, lang: str, title: str, **_kwargs):
        sent_files.append((path, chat_id, title))
        return "video"

    async def fake_record_download_safe(url, platform, format_id, size_mb, *_args, **_kwargs):
        recorded.append((url, platform, format_id, size_mb))

    monkeypatch.setattr("workers.tasks.get_bot", lambda: fake_bot)
    monkeypatch.setattr("workers.tasks.download", fake_download)
    monkeypatch.setattr("workers.tasks.send_file", fake_send_file)
    monkeypatch.setattr("workers.tasks._record_download_safe", fake_record_download_safe)
    monkeypatch.setattr("workers.tasks._platform_download_timeout_seconds", lambda _platform: 0)
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

    assert fake_bot.deleted == [(1, 3)]
    assert sent_files[0][1:] == (1, "direct")
    assert recorded[0][:3] == ("https://vt.tiktok.com/ZSxv29fme/", "tiktok", "best")


def test_download_task_rejects_files_over_video_limit(monkeypatch):
    fake_bot = FakeBot()
    sent_files: list[tuple] = []
    recorded: list[tuple[str, str, str, float, object]] = []

    def fake_download(url: str, output_dir: Path, format_id: str):
        path = output_dir / "video.mp4"
        with path.open("wb") as handle:
            handle.truncate(51 * 1024 * 1024)
        return {"title": "large"}

    async def fake_send_file(*args, **kwargs):
        sent_files.append((args, kwargs))
        return "video"

    async def fake_record_download_safe(url, platform, format_id, size_mb, status, *_args, **_kwargs):
        recorded.append((url, platform, format_id, size_mb, status))

    monkeypatch.setattr("workers.tasks.get_bot", lambda: fake_bot)
    monkeypatch.setattr("workers.tasks.download", fake_download)
    monkeypatch.setattr("workers.tasks.send_file", fake_send_file)
    monkeypatch.setattr("workers.tasks._record_download_safe", fake_record_download_safe)
    monkeypatch.setattr("workers.tasks._platform_download_timeout_seconds", lambda _platform: 0)
    monkeypatch.setattr("workers.tasks.release_user_lock_sync", lambda *_args, **_kwargs: None)

    download_and_send_task.run(
        url="https://x.com/user/status/1234567890123456789",
        format_id="best",
        platform="x",
        chat_id=1,
        user_id=2,
        message_id=3,
        lang="en",
    )

    assert fake_bot.deleted == []
    assert fake_bot.documents == []
    assert sent_files == []
    assert "50 MB" in fake_bot.edited[-1][2]
    assert "can't send" in fake_bot.edited[-1][2]
    assert recorded[0][4] == DownloadStatus.FAILED


def test_download_task_times_out_slow_download(monkeypatch):
    fake_bot = FakeBot()
    sent_files: list[tuple] = []
    recorded: list[tuple[str, str, str, float, object, str | None]] = []

    async def fake_download_with_timeout(url: str, output_dir: Path, format_id: str, platform: str):
        raise asyncio.TimeoutError

    async def fake_send_file(*args, **kwargs):
        sent_files.append((args, kwargs))

    async def fake_record_download_safe(url, platform, format_id, size_mb, status, *_args, **kwargs):
        recorded.append((url, platform, format_id, size_mb, status, kwargs.get("error")))

    monkeypatch.setattr("workers.tasks.get_bot", lambda: fake_bot)
    monkeypatch.setattr("workers.tasks._download_with_timeout", fake_download_with_timeout)
    monkeypatch.setattr("workers.tasks.send_file", fake_send_file)
    monkeypatch.setattr("workers.tasks._record_download_safe", fake_record_download_safe)
    monkeypatch.setattr("workers.tasks.release_user_lock_sync", lambda *_args, **_kwargs: None)

    download_and_send_task.run(
        url="https://x.com/user/status/1234567890123456789",
        format_id="best",
        platform="x",
        chat_id=1,
        user_id=2,
        message_id=3,
        lang="en",
    )

    assert fake_bot.deleted == []
    assert sent_files == []
    assert "try again later" in fake_bot.edited[-1][2]
    assert recorded[0][4] == DownloadStatus.FAILED


def test_download_task_processes_youtube_with_quality_and_ratio(monkeypatch):
    fake_bot = FakeBot()
    sent_files: list[tuple[Path, int, str]] = []
    processed: list[tuple[str, int]] = []

    def fake_download(url: str, output_dir: Path, format_id: str):
        assert "height<=720" in format_id
        (output_dir / "video.mp4").write_bytes(b"video")
        return {"title": "youtube-test"}

    def fake_apply_aspect_ratio(path: Path, aspect_ratio: str, quality: int):
        processed.append((aspect_ratio, quality))
        output = path.with_name("video_16_9.mp4")
        output.write_bytes(b"processed")
        return output

    async def fake_send_file(bot, path: Path, chat_id: int, lang: str, title: str, **_kwargs):
        sent_files.append((path, chat_id, title))
        return "video"

    async def fake_record_download_safe(url, platform, format_id, size_mb, *_args, **_kwargs):
        assert platform == "youtube"
        assert "q720" in format_id
        assert "r16_9" in format_id

    monkeypatch.setattr("workers.tasks.get_bot", lambda: fake_bot)
    monkeypatch.setattr("workers.tasks.download", fake_download)
    monkeypatch.setattr("workers.tasks.apply_aspect_ratio", fake_apply_aspect_ratio)
    monkeypatch.setattr("workers.tasks.send_file", fake_send_file)
    monkeypatch.setattr("workers.tasks._record_download_safe", fake_record_download_safe)
    monkeypatch.setattr("workers.tasks._platform_download_timeout_seconds", lambda _platform: 0)
    monkeypatch.setattr("workers.tasks.release_user_lock_sync", lambda *_args, **_kwargs: None)

    download_and_send_task.run(
        url="https://www.youtube.com/watch?v=dQw4w9WgXcQ",
        platform="youtube",
        chat_id=1,
        user_id=2,
        message_id=3,
        lang="ru",
        quality=720,
        aspect_ratio="16_9",
    )

    assert processed == [("16_9", 720)]
    assert sent_files[0][0].name == "video_16_9.mp4"

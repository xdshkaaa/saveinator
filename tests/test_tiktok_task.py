import time
from pathlib import Path

from db.models import DownloadStatus
from workers.tiktok_task import tiktok_download_task
from workers.tiktok_downloader import TikTokPostType, TikTokDownloadResult


class FakeBot:
    def __init__(self):
        self.deleted: list[tuple[int, int]] = []
        self.edited: list[tuple[int, int, str]] = []
        self.messages: list[tuple[int, str]] = []
        self.photos: list[tuple] = []
        self.media_groups: list[tuple] = []
        self.audios: list[tuple] = []

    async def edit_message_text(self, chat_id: int, message_id: int, text: str):
        self.edited.append((chat_id, message_id, text))

    async def delete_message(self, chat_id: int, message_id: int):
        self.deleted.append((chat_id, message_id))

    async def send_message(self, chat_id: int, text: str):
        self.messages.append((chat_id, text))

    async def send_photo(self, chat_id, photo, caption=None):
        self.photos.append((chat_id, photo, caption))

    async def send_media_group(self, chat_id, media):
        self.media_groups.append((chat_id, media))

    async def send_audio(self, chat_id, audio):
        self.audios.append((chat_id, audio))

    async def send_document(self, chat_id, document, caption=None):
        pass


def _patch_tiktok_runtime_settings(
    monkeypatch,
    *,
    carousel_max_items: int = 20,
    carousel_audio_enabled: int = 1,
):
    def fake_get_runtime_int(key, default=None):
        values = {
            "tiktok.carousel_max_items": carousel_max_items,
            "tiktok.carousel_audio_enabled": carousel_audio_enabled,
        }
        return values.get(key, default if default is not None else 0)

    monkeypatch.setattr("workers.tiktok_task.get_runtime_int", fake_get_runtime_int)


def test_tiktok_task_handles_video(monkeypatch):
    fake_bot = FakeBot()
    recorded: list[tuple] = []

    def fake_download(url, output_dir, **kwargs):
        (output_dir / "video.mp4").write_bytes(b"video" * 1024 * 100)
        return TikTokDownloadResult(
            source_url=url,
            resolved_url=url,
            post_type=TikTokPostType.VIDEO,
            title="test video",
            author="testuser",
            video_path=str(output_dir / "video.mp4"),
        )

    async def fake_send_file(bot, path, chat_id, lang, title, **kwargs):
        return "video"

    async def fake_record(url, platform, fmt, size, status, *_args, **kwargs):
        recorded.append((url, platform, fmt, size, status))

    monkeypatch.setattr("workers.tiktok_task.get_bot", lambda: fake_bot)
    monkeypatch.setattr("workers.tiktok_task.download_tiktok", fake_download)
    monkeypatch.setattr("workers.tiktok_task.send_file", fake_send_file)
    monkeypatch.setattr("workers.tiktok_task._record_download_safe", fake_record)
    monkeypatch.setattr("workers.tiktok_task.platform_download_timeout_seconds", lambda _: 0)
    monkeypatch.setattr("workers.tiktok_task.platform_max_file_mb", lambda _: 500)
    monkeypatch.setattr("workers.tiktok_task.release_user_lock_sync", lambda *a, **kw: None)
    _patch_tiktok_runtime_settings(monkeypatch)

    tiktok_download_task.run(
        url="https://www.tiktok.com/@user/video/123",
        chat_id=1,
        user_id=2,
        message_id=3,
        lang="en",
        lock_token="test-token",
    )

    assert fake_bot.deleted == [(1, 3)]
    assert len(recorded) == 1
    assert recorded[0][4] == DownloadStatus.COMPLETED


def test_tiktok_task_handles_carousel(monkeypatch, tmp_path):
    fake_bot = FakeBot()
    recorded: list[tuple] = []
    download_kwargs: list[dict] = []

    img_paths = []
    for i in range(3):
        p = tmp_path / f"image_{i}.jpg"
        p.write_bytes(b"img")
        img_paths.append(str(p))

    def fake_download(url, output_dir, **kwargs):
        download_kwargs.append(kwargs)
        return TikTokDownloadResult(
            source_url=url,
            resolved_url=url,
            post_type=TikTokPostType.CAROUSEL,
            title="test carousel",
            author="testuser",
            images=img_paths,
            audio=None,
        )

    async def fake_record(url, platform, fmt, size, status, *_args, **kwargs):
        recorded.append((url, platform, fmt, size, status))

    monkeypatch.setattr("workers.tiktok_task.get_bot", lambda: fake_bot)
    monkeypatch.setattr("workers.tiktok_task.download_tiktok", fake_download)
    async def fake_send_carousel(*a, **kw):
        return True

    monkeypatch.setattr("workers.tiktok_task.send_carousel", fake_send_carousel)
    monkeypatch.setattr("workers.tiktok_task._record_download_safe", fake_record)
    monkeypatch.setattr("workers.tiktok_task.platform_download_timeout_seconds", lambda _: 0)
    monkeypatch.setattr("workers.tiktok_task.platform_max_file_mb", lambda _: 500)
    monkeypatch.setattr("workers.tiktok_task.release_user_lock_sync", lambda *a, **kw: None)
    _patch_tiktok_runtime_settings(
        monkeypatch, carousel_max_items=2, carousel_audio_enabled=0
    )

    tiktok_download_task.run(
        url="https://www.tiktok.com/@user/photo/123",
        chat_id=1,
        user_id=2,
        message_id=3,
        lang="en",
        lock_token="test-token",
    )

    assert fake_bot.deleted == [(1, 3)]
    assert len(recorded) == 1
    assert recorded[0][1] == "tiktok"
    assert recorded[0][2] == "carousel"
    assert download_kwargs == [{"max_images": 2, "audio_enabled": False}]


def test_tiktok_task_handles_timeout(monkeypatch):
    fake_bot = FakeBot()
    recorded: list[tuple] = []

    def fake_download_timeout(*args, **kwargs):
        time.sleep(0.05)
        return TikTokDownloadResult(
            source_url=args[0],
            resolved_url=args[0],
            post_type=TikTokPostType.UNKNOWN,
        )

    async def fake_record(url, platform, fmt, size, status, *_args, **kwargs):
        recorded.append((url, platform, fmt, size, status, kwargs.get("error")))

    monkeypatch.setattr("workers.tiktok_task.get_bot", lambda: fake_bot)
    monkeypatch.setattr("workers.tiktok_task.download_tiktok", fake_download_timeout)
    monkeypatch.setattr("workers.tiktok_task.platform_download_timeout_seconds", lambda _: 0.01)
    monkeypatch.setattr("workers.tiktok_task._record_download_safe", fake_record)
    monkeypatch.setattr("workers.tiktok_task.release_user_lock_sync", lambda *a, **kw: None)
    _patch_tiktok_runtime_settings(monkeypatch)

    tiktok_download_task.run(
        url="https://www.tiktok.com/@user/video/123",
        chat_id=1,
        user_id=2,
        message_id=3,
        lang="en",
        lock_token="test-token",
    )

    assert fake_bot.deleted == []
    assert "try again later" in fake_bot.edited[-1][2]
    assert recorded[0][4] == DownloadStatus.FAILED


def test_tiktok_task_releases_lock_on_exception(monkeypatch):
    fake_bot = FakeBot()
    released: list[bool] = []

    def fake_release(user_id, token, scenario):
        released.append(True)

    def fake_download(url, output_dir, **kwargs):
        raise RuntimeError("unexpected")

    monkeypatch.setattr("workers.tiktok_task.get_bot", lambda: fake_bot)
    monkeypatch.setattr("workers.tiktok_task.download_tiktok", fake_download)
    monkeypatch.setattr("workers.tiktok_task.platform_download_timeout_seconds", lambda _: 0)
    monkeypatch.setattr("workers.tiktok_task.release_user_lock_sync", fake_release)
    _patch_tiktok_runtime_settings(monkeypatch)

    tiktok_download_task.run(
        url="https://www.tiktok.com/@user/video/123",
        chat_id=1,
        user_id=2,
        message_id=3,
        lang="en",
        lock_token="test-token",
    )

    assert len(released) == 1


def test_tiktok_task_cleans_up_temp_dir(monkeypatch):
    fake_bot = FakeBot()
    temp_dirs: list[Path] = []

    def fake_download(url, output_dir, **kwargs):
        temp_dirs.append(output_dir)
        (output_dir / "video.mp4").write_bytes(b"video")
        return TikTokDownloadResult(
            source_url=url,
            resolved_url=url,
            post_type=TikTokPostType.VIDEO,
            title="test",
            video_path=str(output_dir / "video.mp4"),
        )

    async def fake_send_file(bot, path, chat_id, lang, title, **kwargs):
        return "video"

    async def fake_record(*args, **kwargs):
        pass

    monkeypatch.setattr("workers.tiktok_task.get_bot", lambda: fake_bot)
    monkeypatch.setattr("workers.tiktok_task.download_tiktok", fake_download)
    monkeypatch.setattr("workers.tiktok_task.send_file", fake_send_file)
    monkeypatch.setattr("workers.tiktok_task._record_download_safe", fake_record)
    monkeypatch.setattr("workers.tiktok_task.platform_download_timeout_seconds", lambda _: 0)
    monkeypatch.setattr("workers.tiktok_task.platform_max_file_mb", lambda _: 500)
    monkeypatch.setattr("workers.tiktok_task.release_user_lock_sync", lambda *a, **kw: None)
    _patch_tiktok_runtime_settings(monkeypatch)

    tiktok_download_task.run(
        url="https://www.tiktok.com/@user/video/123",
        chat_id=1,
        user_id=2,
        message_id=3,
        lang="en",
        lock_token="test-token",
    )

    # Temp dir should be cleaned up after task completes
    for d in temp_dirs:
        assert not d.exists() or not list(d.iterdir())

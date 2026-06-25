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
    monkeypatch.setattr("workers.tasks._has_audio_stream", lambda _path: True)
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

    async def fake_download_with_timeout(*_args, **_kwargs):
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


def test_x_reply_normal_tweet_without_x_status_id_unchanged(monkeypatch):
    """A normal X/Twitter link (no x_status_id) downloads fine and doesn't use reply filter."""
    fake_bot = FakeBot()
    sent_files: list[tuple] = []
    recorded: list[tuple] = []
    used_reply_filter = False

    orig_download = None

    def fake_download(url: str, output_dir: Path, format_id: str):
        nonlocal used_reply_filter, orig_download
        from workers.downloader import download as orig
        orig_download = orig
        if url == "https://x.com/user/status/1234567890123456789":
            (output_dir / "video.mp4").write_bytes(b"video")
            return {"title": "normal-tweet"}
        return orig_download(url, output_dir, format_id)

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
    monkeypatch.setattr("workers.tasks._has_audio_stream", lambda _path: True)
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

    assert fake_bot.deleted == [(1, 3)]
    assert sent_files[0][1:] == (1, "normal-tweet")


def test_x_reply_without_media_returns_no_media_message(monkeypatch):
    """When the target reply has no media, show no-media message without fallback."""
    fake_bot = FakeBot()
    recorded: list[tuple] = []

    async def fake_download_with_timeout(url, output_dir, format_id, platform, x_status_id=None):
        from workers.downloader import XTargetReplyNoMediaError
        raise XTargetReplyNoMediaError("no media")

    async def fake_record_download_safe(url, platform, format_id, size_mb, status, *_args, **_kwargs):
        recorded.append((url, platform, format_id, size_mb, status))

    monkeypatch.setattr("workers.tasks.get_bot", lambda: fake_bot)
    monkeypatch.setattr("workers.tasks._download_with_timeout", fake_download_with_timeout)
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
        x_status_id="1234567890123456789",
    )

    assert "no media found" in fake_bot.edited[-1][2].lower()
    assert recorded[0][4] == DownloadStatus.FAILED


def test_x_reply_not_found_also_shows_no_media_message(monkeypatch):
    """When the target reply is not found in the thread, show same no-media message."""
    fake_bot = FakeBot()
    recorded: list[tuple] = []

    async def fake_download_with_timeout(url, output_dir, format_id, platform, x_status_id=None):
        from workers.downloader import XTargetReplyNotFoundError
        raise XTargetReplyNotFoundError("not found")

    async def fake_record_download_safe(url, platform, format_id, size_mb, status, *_args, **_kwargs):
        recorded.append((url, platform, format_id, size_mb, status))

    monkeypatch.setattr("workers.tasks.get_bot", lambda: fake_bot)
    monkeypatch.setattr("workers.tasks._download_with_timeout", fake_download_with_timeout)
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
        x_status_id="1234567890123456789",
    )

    assert "no media found" in fake_bot.edited[-1][2].lower()
    assert recorded[0][4] == DownloadStatus.FAILED


def test_x_reply_downloads_target_reply_media(monkeypatch):
    """A reply link with x_status_id passes it through and downloads successfully."""
    fake_bot = FakeBot()
    sent_files: list[tuple] = []
    call_kwargs: dict = {}

    async def fake_download_with_timeout(url, output_dir, format_id, platform, x_status_id=None):
        call_kwargs["x_status_id"] = x_status_id
        (output_dir / "video.mp4").write_bytes(b"video")
        return {"title": "reply-video"}

    async def fake_send_file(bot, path, chat_id, lang, title, **_kwargs):
        sent_files.append((path, chat_id, title))
        return "video"

    async def fake_record_download_safe(*args, **_kwargs):
        pass

    monkeypatch.setattr("workers.tasks.get_bot", lambda: fake_bot)
    monkeypatch.setattr("workers.tasks._download_with_timeout", fake_download_with_timeout)
    monkeypatch.setattr("workers.tasks.send_file", fake_send_file)
    monkeypatch.setattr("workers.tasks._record_download_safe", fake_record_download_safe)
    monkeypatch.setattr("workers.tasks._platform_download_timeout_seconds", lambda _platform: 0)
    monkeypatch.setattr("workers.tasks._has_audio_stream", lambda _path: True)
    monkeypatch.setattr("workers.tasks.release_user_lock_sync", lambda *_args, **_kwargs: None)

    download_and_send_task.run(
        url="https://x.com/user/status/9876543210987654321",
        format_id="best",
        platform="x",
        chat_id=1,
        user_id=2,
        message_id=3,
        lang="en",
        x_status_id="9876543210987654321",
    )

    assert call_kwargs.get("x_status_id") == "9876543210987654321"
    assert fake_bot.deleted == [(1, 3)]
    assert sent_files[0][1:] == (1, "reply-video")


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


def test_download_task_youtube_shorts_uses_vertical_format(monkeypatch):
    fake_bot = FakeBot()
    processed: list[tuple[str, int]] = []

    def fake_download(url: str, output_dir: Path, format_id: str):
        assert "width<=1080" in format_id
        (output_dir / "video.mp4").write_bytes(b"video")
        return {"title": "shorts-test"}

    def fake_apply_aspect_ratio(path: Path, aspect_ratio: str, quality: int):
        processed.append((aspect_ratio, quality))
        output = path.with_name("video_9_16.mp4")
        output.write_bytes(b"processed")
        return output

    async def fake_send_file(bot, path: Path, chat_id: int, lang: str, title: str, **_kwargs):
        return "video"

    async def fake_record_download_safe(*_args, **_kwargs):
        return None

    monkeypatch.setattr("workers.tasks.get_bot", lambda: fake_bot)
    monkeypatch.setattr("workers.tasks.download", fake_download)
    monkeypatch.setattr("workers.tasks.apply_aspect_ratio", fake_apply_aspect_ratio)
    monkeypatch.setattr("workers.tasks.send_file", fake_send_file)
    monkeypatch.setattr("workers.tasks._record_download_safe", fake_record_download_safe)
    monkeypatch.setattr("workers.tasks._platform_download_timeout_seconds", lambda _platform: 0)
    monkeypatch.setattr("workers.tasks.release_user_lock_sync", lambda *_args, **_kwargs: None)

    download_and_send_task.run(
        url="https://www.youtube.com/shorts/PuZXo75tdK8?feature=share",
        platform="youtube",
        chat_id=1,
        user_id=2,
        message_id=3,
        lang="ru",
        quality=1080,
        aspect_ratio="9_16",
    )

    assert processed == [("9_16", 1080)]


def test_x_single_image_sent_as_photo(monkeypatch):
    fake_bot = FakeBot()
    sent_files: list[tuple] = []
    recorded: list[tuple] = []

    def fake_download(url: str, output_dir: Path, format_id: str):
        (output_dir / "photo.jpg").write_bytes(b"image-data")
        return {"title": "x-photo"}

    async def fake_send_file(bot, path, chat_id, lang, title, **_kwargs):
        sent_files.append((path.suffix, chat_id, title, _kwargs.get("media_type")))
        return "photo"

    async def fake_record_download_safe(*args, **_kwargs):
        recorded.append(args)

    monkeypatch.setattr("workers.tasks.get_bot", lambda: fake_bot)
    monkeypatch.setattr("workers.tasks.download", fake_download)
    monkeypatch.setattr("workers.tasks.send_file", fake_send_file)
    monkeypatch.setattr("workers.tasks._record_download_safe", fake_record_download_safe)
    monkeypatch.setattr("workers.tasks._platform_download_timeout_seconds", lambda _platform: 0)
    monkeypatch.setattr("workers.tasks._has_audio_stream", lambda _path: True)
    monkeypatch.setattr("workers.tasks.release_user_lock_sync", lambda *_args, **_kwargs: None)

    download_and_send_task.run(
        url="https://x.com/user/status/2069236979384881505",
        format_id="best",
        platform="x",
        chat_id=1,
        user_id=2,
        message_id=3,
        lang="en",
    )

    assert fake_bot.deleted == [(1, 3)]
    assert len(sent_files) == 1
    assert sent_files[0] == (".jpg", 1, "x-photo", "image")


def test_x_multiple_images_all_sent(monkeypatch):
    fake_bot = FakeBot()
    sent_files: list[tuple] = []
    recorded: list[tuple] = []

    def fake_download(url: str, output_dir: Path, format_id: str):
        (output_dir / "photo_1.jpg").write_bytes(b"image-1")
        (output_dir / "photo_2.jpg").write_bytes(b"image-2")
        (output_dir / "photo_3.jpg").write_bytes(b"image-3")
        return {"title": "x-album"}

    async def fake_send_file(bot, path, chat_id, lang, title, **_kwargs):
        sent_files.append((path.name, _kwargs.get("media_type")))
        return "photo"

    async def fake_record_download_safe(*args, **_kwargs):
        recorded.append(args)

    monkeypatch.setattr("workers.tasks.get_bot", lambda: fake_bot)
    monkeypatch.setattr("workers.tasks.download", fake_download)
    monkeypatch.setattr("workers.tasks.send_file", fake_send_file)
    monkeypatch.setattr("workers.tasks._record_download_safe", fake_record_download_safe)
    monkeypatch.setattr("workers.tasks._platform_download_timeout_seconds", lambda _platform: 0)
    monkeypatch.setattr("workers.tasks.release_user_lock_sync", lambda *_args, **_kwargs: None)

    download_and_send_task.run(
        url="https://x.com/user/status/2069236979384881505",
        format_id="best",
        platform="x",
        chat_id=1,
        user_id=2,
        message_id=3,
        lang="en",
    )

    assert fake_bot.deleted == [(1, 3)]
    assert len(sent_files) == 3
    for name, media_type in sent_files:
        assert media_type == "image"
    assert recorded


def test_x_image_does_not_require_video_file(monkeypatch):
    fake_bot = FakeBot()
    sent_files: list = []

    def fake_download(url: str, output_dir: Path, format_id: str):
        (output_dir / "photo.jpg").write_bytes(b"image-data")
        return {"title": "x-image-only"}

    async def fake_send_file(*args, **kwargs):
        sent_files.append(kwargs.get("media_type"))
        return "photo"

    async def fake_record_download_safe(*_args, **_kwargs):
        pass

    monkeypatch.setattr("workers.tasks.get_bot", lambda: fake_bot)
    monkeypatch.setattr("workers.tasks.download", fake_download)
    monkeypatch.setattr("workers.tasks.send_file", fake_send_file)
    monkeypatch.setattr("workers.tasks._record_download_safe", fake_record_download_safe)
    monkeypatch.setattr("workers.tasks._platform_download_timeout_seconds", lambda _platform: 0)
    monkeypatch.setattr("workers.tasks.release_user_lock_sync", lambda *_args, **_kwargs: None)

    download_and_send_task.run(
        url="https://x.com/user/status/2069236979384881505",
        format_id="best",
        platform="x",
        chat_id=1,
        user_id=2,
        message_id=3,
        lang="en",
    )

    # No "video not found" error — message was deleted (success).
    # The only edit is the "Downloading..." status message (not an error).
    assert fake_bot.deleted == [(1, 3)]
    assert len(fake_bot.edited) == 1  # only "Downloading..."
    assert sent_files == ["image"]


def test_x_video_still_uses_video_flow(monkeypatch):
    fake_bot = FakeBot()
    sent_files: list[tuple] = []
    recorded: list[tuple] = []

    def fake_download(url: str, output_dir: Path, format_id: str):
        # Both image and video files in same tweet
        (output_dir / "photo.jpg").write_bytes(b"image-data")
        (output_dir / "video.mp4").write_bytes(b"video-data")
        return {"title": "x-video-with-image"}

    async def fake_send_file(bot, path, chat_id, lang, title, **_kwargs):
        sent_files.append((path.suffix, _kwargs.get("media_type")))
        return "video"

    async def fake_record_download_safe(*args, **_kwargs):
        recorded.append(args)

    monkeypatch.setattr("workers.tasks.get_bot", lambda: fake_bot)
    monkeypatch.setattr("workers.tasks.download", fake_download)
    monkeypatch.setattr("workers.tasks.send_file", fake_send_file)
    monkeypatch.setattr("workers.tasks._record_download_safe", fake_record_download_safe)
    monkeypatch.setattr("workers.tasks._platform_download_timeout_seconds", lambda _platform: 0)
    monkeypatch.setattr("workers.tasks._has_audio_stream", lambda _path: True)
    monkeypatch.setattr("workers.tasks.release_user_lock_sync", lambda *_args, **_kwargs: None)

    download_and_send_task.run(
        url="https://x.com/user/status/2069236979384881505",
        format_id="best",
        platform="x",
        chat_id=1,
        user_id=2,
        message_id=3,
        lang="en",
    )

    assert fake_bot.deleted == [(1, 3)]
    # Video flow: one file sent (the mp4), not the jpg
    assert len(sent_files) == 1
    # media_type is None (not "animation") because _has_audio_stream is mocked to True
    assert sent_files[0] == (".mp4", None)
    assert len(sent_files) == 1


def test_x_gif_detected_as_animation(monkeypatch):
    """An X/Twitter GIF (silent mp4) is detected and passed with media_type='animation'."""
    fake_bot = FakeBot()
    sent_files: list[tuple] = []
    recorded: list[tuple] = []

    def fake_download(url: str, output_dir: Path, format_id: str):
        (output_dir / "gif.mp4").write_bytes(b"gif-data")
        return {"title": "x-gif"}

    async def fake_send_file(bot, path, chat_id, lang, title, **_kwargs):
        sent_files.append((path.suffix, _kwargs.get("media_type")))
        return "animation"

    async def fake_record_download_safe(*args, **_kwargs):
        recorded.append(args)

    monkeypatch.setattr("workers.tasks.get_bot", lambda: fake_bot)
    monkeypatch.setattr("workers.tasks.download", fake_download)
    monkeypatch.setattr("workers.tasks.send_file", fake_send_file)
    monkeypatch.setattr("workers.tasks._record_download_safe", fake_record_download_safe)
    monkeypatch.setattr("workers.tasks._platform_download_timeout_seconds", lambda _platform: 0)
    # GIF: no audio stream → _has_audio_stream returns False
    monkeypatch.setattr("workers.tasks._has_audio_stream", lambda _path: False)
    monkeypatch.setattr("workers.tasks.release_user_lock_sync", lambda *_args, **_kwargs: None)

    download_and_send_task.run(
        url="https://x.com/user/status/2069196942534439213",
        format_id="best",
        platform="x",
        chat_id=1,
        user_id=2,
        message_id=3,
        lang="en",
    )

    assert fake_bot.deleted == [(1, 3)]
    assert len(sent_files) == 1
    # .mp4 with no audio → media_type="animation"
    assert sent_files[0] == (".mp4", "animation")

from pathlib import Path

from db.models import DownloadStatus
from bot.services.pinterest_models import PinterestDownloadResult, PinterestMediaItem, PinterestUrlType
from workers.pinterest_task import pinterest_download_task


class FakeBot:
    def __init__(self):
        self.deleted: list[tuple[int, int]] = []
        self.edited: list[tuple[int, int, str]] = []
        self.sent: list[tuple] = []
        self.messages: list[tuple] = []

    async def edit_message_text(self, chat_id: int, message_id: int, text: str):
        self.edited.append((chat_id, message_id, text))

    async def delete_message(self, chat_id: int, message_id: int):
        self.deleted.append((chat_id, message_id))

    async def send_photo(self, *args, **kwargs):
        self.sent.append(("photo", args, kwargs))

    async def send_video(self, *args, **kwargs):
        self.sent.append(("video", args, kwargs))

    async def send_document(self, *args, **kwargs):
        self.sent.append(("document", args, kwargs))

    async def send_message(self, chat_id: int, text: str):
        self.messages.append((chat_id, text))


def _result_from_paths(url: str, paths: list[Path]) -> PinterestDownloadResult:
    items = []
    for path in paths:
        items.append(
            PinterestMediaItem(
                source_url=url,
                media_type="image",
                title=path.stem,
                description=None,
                original_media_url="https://i.pinimg.com/test.jpg",
                file_path=str(path),
                file_size=path.stat().st_size,
            )
        )
    return PinterestDownloadResult(
        url=url,
        url_type=PinterestUrlType.BOARD if "/board" in url else PinterestUrlType.PIN,
        items=items,
    )


def test_pinterest_task_sends_single_image(monkeypatch):
    fake_bot = FakeBot()
    sent_files: list[tuple[Path, int, str]] = []
    recorded: list[tuple[str, str, float]] = []

    def fake_download_pinterest(url, output_dir, max_items):
        f = output_dir / "image.jpg"
        f.write_bytes(b"image data")
        return _result_from_paths(url, [f])

    async def fake_send_file(bot, path, chat_id, lang, title, media_type=None):
        sent_files.append((path, chat_id, title, media_type))

    async def fake_record_download_safe(url, platform, format_id, size_mb, status, *_a, **_k):
        recorded.append((url, platform, size_mb))

    monkeypatch.setattr("workers.pinterest_task._get_bot", lambda: fake_bot)
    monkeypatch.setattr(
        "workers.pinterest_task.download_pinterest", fake_download_pinterest
    )
    monkeypatch.setattr("workers.pinterest_task.send_file", fake_send_file)
    monkeypatch.setattr(
        "workers.pinterest_task._record_download_safe", fake_record_download_safe
    )

    pinterest_download_task.run(
        url="https://www.pinterest.com/pin/123/",
        chat_id=10,
        user_id=20,
        message_id=30,
        lang="en",
    )

    assert fake_bot.deleted == [(10, 30)]
    assert len(sent_files) == 1
    assert sent_files[0][1] == 10
    assert recorded[0][1] == "pinterest"


def test_pinterest_task_sends_video_with_media_type(monkeypatch):
    fake_bot = FakeBot()
    sent: list[tuple] = []

    def fake_download_pinterest(url, output_dir, max_items):
        f = output_dir / "clip.mp4"
        f.write_bytes(b"video-data")
        return PinterestDownloadResult(
            url=url,
            url_type=PinterestUrlType.PIN,
            items=[
                PinterestMediaItem(
                    source_url=url,
                    media_type="video",
                    title="Clip",
                    description=None,
                    original_media_url="https://v.pinimg.com/clip.mp4",
                    file_path=str(f),
                    file_size=len(b"video-data"),
                )
            ],
        )

    async def fake_send_file(bot, path, chat_id, lang, title, media_type=None):
        sent.append(media_type)

    async def fake_record_download_safe(*_a, **_k):
        pass

    monkeypatch.setattr("workers.pinterest_task._get_bot", lambda: fake_bot)
    monkeypatch.setattr(
        "workers.pinterest_task.download_pinterest", fake_download_pinterest
    )
    monkeypatch.setattr("workers.pinterest_task.send_file", fake_send_file)
    monkeypatch.setattr(
        "workers.pinterest_task._record_download_safe", fake_record_download_safe
    )

    pinterest_download_task.run(
        url="https://www.pinterest.com/pin/123/",
        chat_id=10,
        user_id=20,
        message_id=30,
    )

    assert sent == ["video"]


def test_pinterest_task_handles_no_media(monkeypatch):
    fake_bot = FakeBot()

    def fake_download_pinterest(url, output_dir, max_items):
        return PinterestDownloadResult(
            url=url,
            url_type=PinterestUrlType.PIN,
            items=[],
        )

    async def fake_record_download_safe(*_a, **_k):
        pass

    monkeypatch.setattr("workers.pinterest_task._get_bot", lambda: fake_bot)
    monkeypatch.setattr(
        "workers.pinterest_task.download_pinterest", fake_download_pinterest
    )
    monkeypatch.setattr(
        "workers.pinterest_task._record_download_safe", fake_record_download_safe
    )

    pinterest_download_task.run(
        url="https://www.pinterest.com/pin/999/",
        chat_id=10,
        user_id=20,
        message_id=30,
    )

    assert fake_bot.deleted == []
    assert "No media" in fake_bot.edited[-1][2]


def test_pinterest_task_handles_generic_error(monkeypatch):
    fake_bot = FakeBot()
    recorded: list[tuple] = []

    def fake_download_pinterest(url, output_dir, max_items):
        raise RuntimeError("network error")

    async def fake_record_download(
        url, platform, format_id, size_mb, status, user_id, chat_id, error=None
    ):
        recorded.append((status, error))

    async def fake_record_download_safe(*_a, **_k):
        pass

    monkeypatch.setattr("workers.pinterest_task._get_bot", lambda: fake_bot)
    monkeypatch.setattr(
        "workers.pinterest_task.download_pinterest", fake_download_pinterest
    )
    monkeypatch.setattr(
        "workers.pinterest_task._record_download", fake_record_download
    )
    monkeypatch.setattr(
        "workers.pinterest_task._record_download_safe", fake_record_download_safe
    )

    pinterest_download_task.run(
        url="https://www.pinterest.com/pin/000/",
        chat_id=10,
        user_id=20,
        message_id=30,
    )

    assert "went wrong" in fake_bot.edited[-1][2]
    assert recorded[0][0] == DownloadStatus.FAILED
    assert "network error" in recorded[0][1]

import asyncio
from dataclasses import dataclass
from unittest.mock import AsyncMock

import pytest

from bot.services.music_download_progress import run_ordered_release_download


@dataclass
class _FakeTrack:
    title: str


@dataclass
class _FakeResult:
    index: int
    track: _FakeTrack
    audio_path: str | None
    error: Exception | None = None


class _FakeStatusMessage:
    def __init__(self):
        self.edited_texts: list[str] = []

    async def edit_text(self, text: str, reply_markup=None):
        self.edited_texts.append(text)


@pytest.mark.asyncio
async def test_run_ordered_release_download_sends_in_index_order():
    status_msg = _FakeStatusMessage()
    sent_order: list[int] = []
    tracks = [_FakeTrack(title=f"Track {index}") for index in range(1, 4)]

    async def download_fn(index: int, track: _FakeTrack, on_download_start) -> _FakeResult:
        await on_download_start(index, track)
        await asyncio.sleep(0.02 * (4 - index))
        return _FakeResult(index=index, track=track, audio_path=f"/tmp/{index}.mp3")

    async def send_fn(result: _FakeResult) -> bool:
        sent_order.append(result.index)
        return True

    sent, results = await run_ordered_release_download(
        tracks=tracks,
        status_msg=status_msg,
        lang="en",
        locale_prefix="spotify",
        cancel_keyboard=None,
        download_fn=download_fn,
        send_fn=send_fn,
    )

    assert sent == 3
    assert sent_order == [1, 2, 3]
    assert len(results) == 3
    assert "Downloading 1/3: Track 1" in status_msg.edited_texts
    assert "Sending 1/3: Track 1" in status_msg.edited_texts
    assert "Sending 3/3: Track 3" in status_msg.edited_texts


@pytest.mark.asyncio
async def test_run_ordered_release_download_skips_failed_track():
    status_msg = _FakeStatusMessage()
    sent_order: list[int] = []
    tracks = [_FakeTrack(title=f"Track {index}") for index in range(1, 4)]

    async def download_fn(index: int, track: _FakeTrack, on_download_start) -> _FakeResult:
        await on_download_start(index, track)
        if index == 2:
            return _FakeResult(index=index, track=track, audio_path=None, error=RuntimeError("fail"))
        return _FakeResult(index=index, track=track, audio_path=f"/tmp/{index}.mp3")

    async def send_fn(result: _FakeResult) -> bool:
        sent_order.append(result.index)
        return True

    sent, results = await run_ordered_release_download(
        tracks=tracks,
        status_msg=status_msg,
        lang="en",
        locale_prefix="spotify",
        cancel_keyboard=None,
        download_fn=download_fn,
        send_fn=send_fn,
    )

    assert sent == 2
    assert sent_order == [1, 3]
    assert len(results) == 3
    assert "Sending 2/3: Track 2" not in status_msg.edited_texts


@pytest.mark.asyncio
async def test_run_ordered_release_download_handles_send_failure():
    status_msg = _FakeStatusMessage()
    tracks = [_FakeTrack(title="Track 1"), _FakeTrack(title="Track 2")]

    async def download_fn(index: int, track: _FakeTrack, on_download_start) -> _FakeResult:
        await on_download_start(index, track)
        return _FakeResult(index=index, track=track, audio_path=f"/tmp/{index}.mp3")

    send_fn = AsyncMock(side_effect=[False, True])

    sent, _results = await run_ordered_release_download(
        tracks=tracks,
        status_msg=status_msg,
        lang="en",
        locale_prefix="soundcloud",
        cancel_keyboard=None,
        download_fn=download_fn,
        send_fn=send_fn,
    )

    assert sent == 1
    assert send_fn.await_count == 2


@pytest.mark.asyncio
async def test_download_start_only_when_download_fn_invokes_callback():
    status_msg = _FakeStatusMessage()
    tracks = [_FakeTrack(title=f"Track {index}") for index in range(1, 4)]
    semaphore = asyncio.Semaphore(1)

    async def download_fn(index: int, track: _FakeTrack, on_download_start) -> _FakeResult:
        async with semaphore:
            await on_download_start(index, track)
            await asyncio.sleep(0.01)
        return _FakeResult(index=index, track=track, audio_path=f"/tmp/{index}.mp3")

    async def send_fn(_result: _FakeResult) -> bool:
        return True

    await run_ordered_release_download(
        tracks=tracks,
        status_msg=status_msg,
        lang="en",
        locale_prefix="spotify",
        cancel_keyboard=None,
        download_fn=download_fn,
        send_fn=send_fn,
    )

    download_statuses = [text for text in status_msg.edited_texts if text.startswith("Downloading")]
    assert download_statuses == [
        "Downloading 1/3: Track 1",
        "Downloading 2/3: Track 2",
        "Downloading 3/3: Track 3",
    ]

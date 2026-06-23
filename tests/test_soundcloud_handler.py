import asyncio
from pathlib import Path
from unittest.mock import AsyncMock

import pytest

from bot.handlers.group import handle_group_message
from bot.services.soundcloud_client import (
    SoundCloudNotFoundError,
    SoundCloudPlaylistTooLargeError,
    SoundCloudTimeoutError,
)
from bot.services.soundcloud_handler import _TrackDownloadResult
from bot.services.soundcloud_models import NormalizedSoundCloudRelease, NormalizedSoundCloudTrack


class FakeUser:
    id = 20


class FakeChat:
    id = 10


class FakeReplyMessage:
    message_id = 99

    def __init__(self):
        self.texts: list[str] = []
        self.photos: list[tuple[str, str | None]] = []
        self.reply_markup = None
        self.edited_texts: list[str] = []

    async def reply(self, text: str, reply_markup=None):
        self.texts.append(text)
        self.reply_markup = reply_markup
        return self

    async def reply_photo(self, photo: str, caption: str | None = None, reply_markup=None):
        self.photos.append((photo, caption))
        self.reply_markup = reply_markup
        return self

    async def edit_text(self, text: str):
        self.edited_texts.append(text)


class FakeBot:
    def __init__(self):
        self.sent_audio: list[dict] = []

    async def send_audio(self, **kwargs):
        self.sent_audio.append(kwargs)


class FakeMessage:
    chat = FakeChat()
    from_user = FakeUser()

    def __init__(self, text: str):
        self.text = text
        self.replies: list[FakeReplyMessage] = []
        self.bot = FakeBot()

    async def reply(self, text: str, reply_markup=None):
        reply = FakeReplyMessage()
        await reply.reply(text, reply_markup=reply_markup)
        self.replies.append(reply)
        return reply

    async def reply_photo(self, photo: str, caption: str | None = None, reply_markup=None):
        reply = FakeReplyMessage()
        await reply.reply_photo(photo, caption=caption, reply_markup=reply_markup)
        self.replies.append(reply)
        return reply


def _sample_track_release() -> NormalizedSoundCloudRelease:
    track = NormalizedSoundCloudTrack(
        source_id="1",
        title="Track One",
        artist="Artist One",
        duration_ms=180000,
        soundcloud_url="https://soundcloud.com/artist/track-one",
        artwork_url="https://example.com/art.jpg",
        genre="Electronic",
        description="",
        created_at="",
        track_number=1,
    )
    return NormalizedSoundCloudRelease(
        source_id="1",
        title="Track One",
        artist="Artist One",
        release_type="track",
        artwork_url="https://example.com/art.jpg",
        soundcloud_url="https://soundcloud.com/artist/track-one",
        tracks=[track],
    )


async def _noop_release(*_args, **_kwargs):
    return None


def _patch_soundcloud_settings(monkeypatch):
    monkeypatch.setattr("bot.handlers.group.settings.soundcloud_enabled", True)
    monkeypatch.setattr("bot.handlers.group.settings.soundcloud_download_enabled", False)
    monkeypatch.setattr("bot.handlers.group.settings.soundcloud_max_tracks", 20)

    async def _acquire_lock(*_args, **_kwargs):
        return "test-lock-token"

    monkeypatch.setattr("bot.handlers.group.acquire_user_lock", _acquire_lock)
    monkeypatch.setattr("bot.services.soundcloud_handler.release_user_lock", _noop_release)


async def test_soundcloud_track_replies_metadata_card_without_download(monkeypatch):
    message = FakeMessage("https://soundcloud.com/artist/track-one")
    release = _sample_track_release()

    _patch_soundcloud_settings(monkeypatch)
    monkeypatch.setattr(
        "bot.services.soundcloud_handler.fetch_release",
        AsyncMock(return_value=release),
    )

    await handle_group_message(message, lang="en")

    assert message.replies
    caption = message.replies[0].photos[0][1]
    assert caption is not None
    assert "Artist One" in caption
    assert "Track One" in caption
    assert message.replies[0].reply_markup is not None


async def test_soundcloud_downloads_tracks_when_enabled(monkeypatch, tmp_path: Path):
    message = FakeMessage("https://soundcloud.com/artist/track-one")
    release = _sample_track_release()
    audio_path = tmp_path / "Track One.mp3"
    audio_path.write_bytes(b"audio")

    _patch_soundcloud_settings(monkeypatch)
    monkeypatch.setattr("bot.handlers.group.settings.soundcloud_download_enabled", True)
    monkeypatch.setattr(
        "bot.services.soundcloud_handler.is_soundcloud_download_enabled",
        lambda _s: True,
    )
    monkeypatch.setattr(
        "bot.services.soundcloud_handler.fetch_release",
        AsyncMock(return_value=release),
    )

    async def _download_one_track(index, track, task_dir, settings):
        return _TrackDownloadResult(
            index=index,
            track=track,
            audio_path=str(audio_path),
            error=None,
        )

    monkeypatch.setattr(
        "bot.services.soundcloud_handler._download_one_track",
        _download_one_track,
    )
    monkeypatch.setattr(
        "bot.services.soundcloud_handler._try_acquire_release_download_lock",
        AsyncMock(return_value=True),
    )
    monkeypatch.setattr(
        "bot.services.soundcloud_handler._release_release_download_lock",
        AsyncMock(),
    )

    class FakeTempDir:
        def __enter__(self):
            return tmp_path

        def __exit__(self, *args):
            return False

    monkeypatch.setattr(
        "bot.services.soundcloud_handler.tempfile_manager",
        lambda _task_id: FakeTempDir(),
    )

    await handle_group_message(message, lang="en")
    await asyncio.sleep(0.05)

    assert len(message.replies) == 2
    assert message.bot.sent_audio
    assert "Finished: 1/1 tracks sent." in message.replies[1].edited_texts[-1]


async def test_soundcloud_not_found(monkeypatch):
    message = FakeMessage("https://soundcloud.com/artist/missing")
    _patch_soundcloud_settings(monkeypatch)
    monkeypatch.setattr(
        "bot.services.soundcloud_handler.fetch_release",
        AsyncMock(side_effect=SoundCloudNotFoundError("missing")),
    )

    await handle_group_message(message, lang="en")

    assert "not found" in message.replies[0].texts[0].lower()


async def test_soundcloud_playlist_too_large(monkeypatch):
    message = FakeMessage("https://soundcloud.com/artist/sets/huge")
    _patch_soundcloud_settings(monkeypatch)
    monkeypatch.setattr(
        "bot.services.soundcloud_handler.fetch_release",
        AsyncMock(side_effect=SoundCloudPlaylistTooLargeError("too large")),
    )

    await handle_group_message(message, lang="en")

    assert "too many tracks" in message.replies[0].texts[0].lower()


async def test_soundcloud_metadata_timeout(monkeypatch):
    message = FakeMessage("https://soundcloud.com/artist/slow")
    _patch_soundcloud_settings(monkeypatch)
    monkeypatch.setattr(
        "bot.services.soundcloud_handler.fetch_release",
        AsyncMock(side_effect=SoundCloudTimeoutError("timeout")),
    )

    await handle_group_message(message, lang="en")

    assert "too long" in message.replies[0].texts[0].lower()

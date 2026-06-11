import asyncio
from pathlib import Path

from bot.handlers.group import handle_group_message
from bot.services.spotify_models import NormalizedSpotifyRelease, NormalizedSpotifyTrack


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


def _sample_release() -> NormalizedSpotifyRelease:
    return NormalizedSpotifyRelease(
        source_id="4aawyAB9rmqOaP8fadcCl4",
        title="Test Album",
        album_type="album",
        artists="Artist One",
        release_date="2021-05-21",
        cover_url="https://i.scdn.co/image/cover.jpg",
        spotify_url="https://open.spotify.com/album/4aawyAB9rmqOaP8fadcCl4",
        tracks=[
            NormalizedSpotifyTrack(
                source_id="111",
                disc_number=1,
                track_number=1,
                title="Track One",
                artists="Artist One",
                duration_ms=180000,
                spotify_url="https://open.spotify.com/track/111",
            )
        ],
    )


async def _noop_release(*_args, **_kwargs):
    return None


async def _noop_extend(*_args, **_kwargs):
    return True


def _patch_spotify_settings(monkeypatch):
    monkeypatch.setattr("bot.handlers.group.settings.spotify_enabled", True)
    monkeypatch.setattr("bot.handlers.group.settings.spotify_client_id", "client-id")
    monkeypatch.setattr("bot.handlers.group.settings.spotify_client_secret", "client-secret")
    monkeypatch.setattr("bot.handlers.group.settings.spotify_download_enabled", False)
    async def _acquire_lock(*_args, **_kwargs):
        return "test-lock-token"

    monkeypatch.setattr("bot.handlers.group.acquire_user_lock", _acquire_lock)
    monkeypatch.setattr("bot.services.spotify_handler.release_user_lock", _noop_release)
    monkeypatch.setattr("bot.services.spotify_handler.extend_user_lock", _noop_extend)


async def test_spotify_album_replies_metadata_card_without_download(monkeypatch):
    delayed: list[dict] = []
    message = FakeMessage("https://open.spotify.com/album/4aawyAB9rmqOaP8fadcCl4?si=abc")
    release = _sample_release()

    _patch_spotify_settings(monkeypatch)
    monkeypatch.setattr(
        "bot.handlers.group.download_and_send_task.delay",
        lambda **kwargs: delayed.append(kwargs),
    )
    monkeypatch.setattr(
        "bot.services.spotify_handler.fetch_release",
        lambda link_type, resource_id, settings: release,
    )

    await handle_group_message(message, lang="en")

    assert not delayed
    assert message.replies
    caption = message.replies[0].photos[0][1]
    assert caption is not None
    assert "Artist One" in caption
    assert "Test Album" in caption
    assert "spotify-dl" not in caption.lower()
    assert "youtube" not in caption.lower()
    assert "Audio download is disabled" in caption
    assert message.replies[0].reply_markup is not None


async def test_spotify_track_replies_metadata_card(monkeypatch):
    message = FakeMessage("https://open.spotify.com/track/0VjIjW4GlUZAMYd2vXMi3b")
    release = NormalizedSpotifyRelease(
        source_id="0VjIjW4GlUZAMYd2vXMi3b",
        title="Blinding Lights",
        album_type="track",
        artists="The Weeknd",
        release_date="2020-03-20",
        cover_url="https://i.scdn.co/image/track-cover.jpg",
        spotify_url="https://open.spotify.com/track/0VjIjW4GlUZAMYd2vXMi3b",
        tracks=[
            NormalizedSpotifyTrack(
                source_id="0VjIjW4GlUZAMYd2vXMi3b",
                disc_number=1,
                track_number=1,
                title="Blinding Lights",
                artists="The Weeknd",
                duration_ms=200000,
                spotify_url="https://open.spotify.com/track/0VjIjW4GlUZAMYd2vXMi3b",
            )
        ],
    )

    _patch_spotify_settings(monkeypatch)
    monkeypatch.setattr(
        "bot.services.spotify_handler.fetch_release",
        lambda link_type, resource_id, settings: release,
    )

    await handle_group_message(message, lang="en")

    caption = message.replies[0].photos[0][1]
    assert caption is not None
    assert "The Weeknd" in caption
    assert "Blinding Lights" in caption
    assert "3:20" in caption


async def test_spotify_downloads_tracks_via_youtube(monkeypatch, tmp_path: Path):
    message = FakeMessage("https://open.spotify.com/album/4aawyAB9rmqOaP8fadcCl4")
    release = _sample_release()
    audio_path = tmp_path / "Track One.mp3"
    audio_path.write_bytes(b"audio")

    _patch_spotify_settings(monkeypatch)
    monkeypatch.setattr("bot.handlers.group.settings.spotify_download_enabled", True)
    monkeypatch.setattr("bot.services.spotify_handler.is_spotify_download_enabled", lambda _s: True)
    monkeypatch.setattr(
        "bot.services.spotify_handler.fetch_release",
        lambda link_type, resource_id, settings: release,
    )
    monkeypatch.setattr(
        "bot.services.spotify_handler.download_track_from_youtube",
        lambda track, output_dir, settings: audio_path,
    )

    class FakeTempDir:
        def __enter__(self):
            return tmp_path

        def __exit__(self, *args):
            return False

    monkeypatch.setattr(
        "bot.services.spotify_handler.tempfile_manager",
        lambda _task_id: FakeTempDir(),
    )

    await handle_group_message(message, lang="en")
    await asyncio.sleep(0.05)

    assert len(message.replies) == 2
    assert message.bot.sent_audio
    assert "Finished: 1/1 tracks sent." in message.replies[1].edited_texts[-1]
    assert "spotify-dl" not in message.replies[1].texts[0].lower()

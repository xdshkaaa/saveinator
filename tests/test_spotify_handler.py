from bot.handlers.group import handle_group_message
from bot.services.spotify_models import SpotifyAlbum, SpotifyTrack


class FakeUser:
    id = 20


class FakeChat:
    id = 10


class FakeReplyMessage:
    def __init__(self):
        self.texts: list[str] = []
        self.photos: list[tuple[str, str | None]] = []
        self.reply_markup = None

    async def reply(self, text: str, reply_markup=None):
        self.texts.append(text)
        self.reply_markup = reply_markup
        return self

    async def reply_photo(self, photo: str, caption: str | None = None, reply_markup=None):
        self.photos.append((photo, caption))
        self.reply_markup = reply_markup
        return self


class FakeMessage:
    chat = FakeChat()
    from_user = FakeUser()

    def __init__(self, text: str):
        self.text = text
        self.replies: list[FakeReplyMessage] = []

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


async def test_spotify_album_replies_metadata_card_without_download(monkeypatch):
    delayed: list[dict] = []
    message = FakeMessage("https://open.spotify.com/album/4aawyAB9rmqOaP8fadcCl4?si=abc")

    album = SpotifyAlbum(
        album_id="4aawyAB9rmqOaP8fadcCl4",
        album_name="Test Album",
        album_type="album",
        artists="Artist One",
        release_date="2021-05-21",
        cover_url="https://i.scdn.co/image/cover.jpg",
        spotify_url="https://open.spotify.com/album/4aawyAB9rmqOaP8fadcCl4",
        tracks=[
            SpotifyTrack(
                disc_number=1,
                track_number=1,
                title="Track One",
                artists="Artist One",
                duration_ms=180000,
                spotify_url="https://open.spotify.com/track/111",
            )
        ],
    )

    monkeypatch.setattr("bot.handlers.group.settings.spotify_enabled", True)
    monkeypatch.setattr("bot.handlers.group.settings.spotify_client_id", "client-id")
    monkeypatch.setattr("bot.handlers.group.settings.spotify_client_secret", "client-secret")
    monkeypatch.setattr(
        "bot.handlers.group.download_and_send_task.delay",
        lambda **kwargs: delayed.append(kwargs),
    )
    monkeypatch.setattr("bot.handlers.group.fetch_album", lambda album_id, settings: album)

    await handle_group_message(message, lang="en")

    assert not delayed
    assert message.replies
    caption = message.replies[0].photos[0][1]
    assert caption is not None
    assert "Test Album" in caption
    assert "downloading Spotify content is not supported" in caption
    assert message.replies[0].reply_markup is not None

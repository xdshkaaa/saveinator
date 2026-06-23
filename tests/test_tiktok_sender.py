import pytest
from pathlib import Path

from bot.services.tiktok_sender import send_carousel


class FakeBot:
    def __init__(self):
        self.photos: list[tuple] = []
        self.media_groups: list[tuple] = []
        self.audios: list[tuple] = []
        self.deleted_messages: list[tuple] = []
        self.fail_media_group: bool = False

    async def send_photo(self, chat_id, photo, caption=None):
        self.photos.append((chat_id, photo, caption))

    async def send_media_group(self, chat_id, media):
        if self.fail_media_group:
            from aiogram.exceptions import TelegramBadRequest
            raise TelegramBadRequest("bad request")
        self.media_groups.append((chat_id, media))

    async def send_audio(self, chat_id, audio):
        self.audios.append((chat_id, audio))

    async def delete_message(self, chat_id, message_id):
        self.deleted_messages.append((chat_id, message_id))


@pytest.fixture
def bot():
    return FakeBot()


@pytest.fixture
def image_paths(tmp_path):
    paths = []
    for i in range(3):
        p = tmp_path / f"image_{i}.jpg"
        p.write_bytes(b"image" + str(i).encode())
        paths.append(str(p))
    return paths


@pytest.fixture
def audio_path(tmp_path):
    p = tmp_path / "audio.mp3"
    p.write_bytes(b"audio")
    return str(p)


@pytest.mark.asyncio
async def test_send_carousel_single_image(bot, tmp_path):
    img = tmp_path / "img.jpg"
    img.write_bytes(b"img")
    result = await send_carousel(bot, 1, [str(img)], None, "en")
    assert result is True
    assert len(bot.photos) == 1


@pytest.mark.asyncio
async def test_send_carousel_album(bot, image_paths):
    result = await send_carousel(bot, 1, image_paths, None, "en")
    assert result is True
    assert len(bot.media_groups) == 1


@pytest.mark.asyncio
async def test_send_carousel_with_audio(bot, image_paths, audio_path):
    result = await send_carousel(bot, 1, image_paths, audio_path, "en")
    assert result is True
    assert len(bot.audios) == 1


@pytest.mark.asyncio
async def test_send_carousel_empty_images(bot):
    result = await send_carousel(bot, 1, [], None, "en")
    assert result is False
    assert len(bot.photos) == 0


@pytest.mark.asyncio
async def test_send_carousel_media_group_fallback(bot, image_paths):
    bot.fail_media_group = True
    result = await send_carousel(bot, 1, image_paths, None, "en")
    assert result is True
    assert len(bot.media_groups) == 0
    assert len(bot.photos) == 3


@pytest.mark.asyncio
async def test_send_carousel_chunks(tmp_path, bot):
    paths = []
    for i in range(12):
        p = tmp_path / f"image_{i}.jpg"
        p.write_bytes(b"img" + str(i).encode())
        paths.append(str(p))

    result = await send_carousel(bot, 1, paths, None, "en")
    assert result is True
    # 12 images = 2 chunks (10 + 2)
    assert len(bot.media_groups) == 2


@pytest.mark.asyncio
async def test_send_carousel_deletes_status_message(bot, image_paths):
    await send_carousel(bot, 1, image_paths, None, "en", status_message_id=42)
    assert (1, 42) in bot.deleted_messages

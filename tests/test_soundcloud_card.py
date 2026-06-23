from types import SimpleNamespace

from bot.services.soundcloud_card import build_soundcloud_card_text, build_soundcloud_keyboard
from bot.services.soundcloud_models import NormalizedSoundCloudRelease, NormalizedSoundCloudTrack


def _settings(**overrides):
    defaults = {
        "soundcloud_download_enabled": False,
    }
    defaults.update(overrides)
    return SimpleNamespace(**defaults)


def _track_release() -> NormalizedSoundCloudRelease:
    track = NormalizedSoundCloudTrack(
        source_id="1",
        title="Midnight City",
        artist="M83",
        duration_ms=242000,
        soundcloud_url="https://soundcloud.com/m83/midnight-city",
        artwork_url="https://example.com/art.jpg",
        genre="Electronic",
        description="",
        created_at="",
        track_number=1,
    )
    return NormalizedSoundCloudRelease(
        source_id="1",
        title="Midnight City",
        artist="M83",
        release_type="track",
        artwork_url="https://example.com/art.jpg",
        soundcloud_url="https://soundcloud.com/m83/midnight-city",
        tracks=[track],
    )


def _playlist_release() -> NormalizedSoundCloudRelease:
    tracks = [
        NormalizedSoundCloudTrack(
            source_id=str(index),
            title=f"Track {index}",
            artist="Artist",
            duration_ms=180000,
            soundcloud_url=f"https://soundcloud.com/artist/track-{index}",
            artwork_url=None,
            genre="",
            description="",
            created_at="",
            track_number=index,
        )
        for index in range(1, 4)
    ]
    return NormalizedSoundCloudRelease(
        source_id="playlist-1",
        title="Summer Mix",
        artist="Artist",
        release_type="playlist",
        artwork_url="https://example.com/cover.jpg",
        soundcloud_url="https://soundcloud.com/artist/sets/summer-mix",
        tracks=tracks,
    )


class TestSoundCloudCard:
    def test_track_card_text(self):
        text = build_soundcloud_card_text(_track_release(), "en", _settings())
        assert "M83" in text
        assert "Midnight City" in text
        assert "4:02" in text
        assert "Electronic" in text
        assert "Audio download is disabled" in text

    def test_playlist_card_text(self):
        text = build_soundcloud_card_text(_playlist_release(), "en", _settings())
        assert "Summer Mix" in text
        assert "Tracks: 3" in text

    def test_open_button(self):
        keyboard = build_soundcloud_keyboard(_track_release(), "en")
        button = keyboard.inline_keyboard[0][0]
        assert button.text == "Open in SoundCloud"
        assert button.url == "https://soundcloud.com/m83/midnight-city"

    def test_no_download_message_hidden_when_enabled(self, monkeypatch):
        monkeypatch.setattr(
            "bot.services.soundcloud_card.is_soundcloud_download_enabled",
            lambda _s: True,
        )
        text = build_soundcloud_card_text(
            _track_release(),
            "en",
            _settings(soundcloud_download_enabled=True),
        )
        assert "Audio download is disabled" not in text

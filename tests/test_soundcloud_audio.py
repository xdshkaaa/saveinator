from pathlib import Path
from unittest.mock import MagicMock

import pytest

from bot.services.soundcloud_audio import (
    SoundCloudAudioError,
    SoundCloudAudioTimeoutError,
    SoundCloudAudioTooLargeError,
    download_track,
    is_soundcloud_download_enabled,
)
from bot.services.soundcloud_models import NormalizedSoundCloudTrack


def _settings(**overrides):
    defaults = {
        "soundcloud_download_enabled": True,
        "soundcloud_dl_output_format": "mp3",
        "soundcloud_max_file_mb": 50,
        "soundcloud_track_timeout_seconds": 30,
    }
    defaults.update(overrides)
    return type("Settings", (), defaults)()


def _track() -> NormalizedSoundCloudTrack:
    return NormalizedSoundCloudTrack(
        source_id="1",
        title="Track One",
        artist="Artist",
        duration_ms=180000,
        soundcloud_url="https://soundcloud.com/artist/track-one",
        artwork_url=None,
        genre="",
        description="",
        created_at="",
        track_number=1,
    )


class TestSoundCloudAudio:
    def test_is_soundcloud_download_enabled_requires_yt_dlp(self, monkeypatch):
        monkeypatch.setattr("bot.services.soundcloud_audio.is_yt_dlp_available", lambda: False)
        assert is_soundcloud_download_enabled(_settings()) is False

        monkeypatch.setattr("bot.services.soundcloud_audio.is_yt_dlp_available", lambda: True)
        assert is_soundcloud_download_enabled(_settings()) is True
        assert is_soundcloud_download_enabled(_settings(soundcloud_download_enabled=False)) is False

    def test_download_track_success(self, monkeypatch, tmp_path: Path):
        audio_file = tmp_path / "Track One.mp3"
        audio_file.write_bytes(b"audio")

        completed = MagicMock(returncode=0, stdout="", stderr="")
        monkeypatch.setattr("bot.services.soundcloud_audio.shutil.which", lambda _: "/usr/bin/yt-dlp")
        monkeypatch.setattr("bot.services.soundcloud_audio.subprocess.run", lambda *args, **kwargs: completed)
        monkeypatch.setattr(
            "bot.services.soundcloud_audio.soundcloud_track_timeout_seconds",
            lambda: 30,
        )
        monkeypatch.setattr(
            "bot.services.soundcloud_audio.soundcloud_max_file_mb",
            lambda: 50,
        )

        result = download_track(_track(), tmp_path, _settings())
        assert result == audio_file

    def test_download_track_timeout(self, monkeypatch, tmp_path: Path):
        import subprocess

        monkeypatch.setattr("bot.services.soundcloud_audio.shutil.which", lambda _: "/usr/bin/yt-dlp")
        monkeypatch.setattr(
            "bot.services.soundcloud_audio.subprocess.run",
            lambda *args, **kwargs: (_ for _ in ()).throw(
                subprocess.TimeoutExpired(cmd="yt-dlp", timeout=30)
            ),
        )
        monkeypatch.setattr(
            "bot.services.soundcloud_audio.soundcloud_track_timeout_seconds",
            lambda: 30,
        )

        with pytest.raises(SoundCloudAudioTimeoutError):
            download_track(_track(), tmp_path, _settings())

    def test_download_track_too_large(self, monkeypatch, tmp_path: Path):
        audio_file = tmp_path / "Track One.mp3"
        audio_file.write_bytes(b"x" * 1024)

        completed = MagicMock(returncode=0, stdout="", stderr="")
        monkeypatch.setattr("bot.services.soundcloud_audio.shutil.which", lambda _: "/usr/bin/yt-dlp")
        monkeypatch.setattr("bot.services.soundcloud_audio.subprocess.run", lambda *args, **kwargs: completed)
        monkeypatch.setattr(
            "bot.services.soundcloud_audio.soundcloud_track_timeout_seconds",
            lambda: 30,
        )
        monkeypatch.setattr(
            "bot.services.soundcloud_audio.soundcloud_max_file_mb",
            lambda: 0,
        )

        with pytest.raises(SoundCloudAudioTooLargeError):
            download_track(_track(), tmp_path, _settings())

    def test_download_track_failure(self, monkeypatch, tmp_path: Path):
        completed = MagicMock(returncode=1, stdout="", stderr="download failed")
        monkeypatch.setattr("bot.services.soundcloud_audio.shutil.which", lambda _: "/usr/bin/yt-dlp")
        monkeypatch.setattr("bot.services.soundcloud_audio.subprocess.run", lambda *args, **kwargs: completed)
        monkeypatch.setattr(
            "bot.services.soundcloud_audio.soundcloud_track_timeout_seconds",
            lambda: 30,
        )

        with pytest.raises(SoundCloudAudioError):
            download_track(_track(), tmp_path, _settings())

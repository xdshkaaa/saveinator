from pathlib import Path

import pytest

from bot.config import Settings
from bot.services.spotify_models import NormalizedSpotifyTrack
from bot.services.youtube_audio import (
    YoutubeAudioError,
    YoutubeAudioNotFoundError,
    build_track_search_query,
    download_track_from_youtube,
    is_spotify_download_enabled,
)


def _settings(**overrides) -> Settings:
    base = {
        "bot_token": "test-token",
        "spotify_download_enabled": True,
        "spotify_track_timeout_seconds": 15,
    }
    base.update(overrides)
    return Settings(**base)


def _track() -> NormalizedSpotifyTrack:
    return NormalizedSpotifyTrack(
        source_id="111",
        title="Track One",
        artists="Artist One",
        duration_ms=180000,
        spotify_url="https://open.spotify.com/track/111",
        disc_number=1,
        track_number=1,
    )


class TestYoutubeAudio:
    def test_build_track_search_query(self):
        assert build_track_search_query(_track()) == "Artist One - Track One"

    def test_is_spotify_download_enabled_requires_yt_dlp(self, monkeypatch):
        monkeypatch.setattr("bot.services.youtube_audio.shutil.which", lambda _: None)
        assert is_spotify_download_enabled(_settings()) is False

        monkeypatch.setattr("bot.services.youtube_audio.shutil.which", lambda _: "/usr/bin/yt-dlp")
        assert is_spotify_download_enabled(_settings()) is True

    def test_raises_when_yt_dlp_missing(self, monkeypatch):
        monkeypatch.setattr("bot.services.youtube_audio.shutil.which", lambda _: None)
        with pytest.raises(YoutubeAudioNotFoundError):
            download_track_from_youtube(_track(), Path("/tmp/out"), _settings())

    def test_download_success(self, monkeypatch, tmp_path: Path):
        monkeypatch.setattr("bot.services.youtube_audio.shutil.which", lambda _: "/usr/bin/yt-dlp")

        def fake_run(command, **kwargs):
            output_dir = Path(command[command.index("-o") + 1]).parent
            (output_dir / "Track One.mp3").write_bytes(b"audio")
            class Result:
                returncode = 0
                stdout = ""
                stderr = ""
            return Result()

        monkeypatch.setattr("bot.services.youtube_audio.subprocess.run", fake_run)

        path = download_track_from_youtube(_track(), tmp_path, _settings())
        assert path.name == "Track One.mp3"

    def test_download_failure(self, monkeypatch, tmp_path: Path):
        monkeypatch.setattr("bot.services.youtube_audio.shutil.which", lambda _: "/usr/bin/yt-dlp")

        def fake_run(command, **kwargs):
            class Result:
                returncode = 1
                stdout = ""
                stderr = "no results"
            return Result()

        monkeypatch.setattr("bot.services.youtube_audio.subprocess.run", fake_run)

        with pytest.raises(YoutubeAudioError, match="no results"):
            download_track_from_youtube(_track(), tmp_path, _settings())

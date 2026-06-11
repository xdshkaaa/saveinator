from pathlib import Path

import pytest

from bot.config import Settings
from bot.services.spotify_dl import (
    SpotifyDlError,
    SpotifyDlNotFoundError,
    SpotifyDlTrack,
    build_spotify_dl_command,
    collect_audio_files,
    is_spotify_download_enabled,
    run_spotify_dl,
)


def _settings(**overrides) -> Settings:
    base = {
        "bot_token": "test-token",
        "spotify_client_id": "client-id",
        "spotify_client_secret": "client-secret",
        "spotify_download_enabled": True,
    }
    base.update(overrides)
    return Settings(**base)


class TestSpotifyDlHelpers:
    def test_build_spotify_dl_command(self):
        settings = _settings()
        command = build_spotify_dl_command(
            "https://open.spotify.com/album/4aawyAB9rmqOaP8fadcCl4",
            Path("/tmp/out"),
            settings,
        )
        assert command[0] == "spotifydl"
        assert "https://open.spotify.com/album/4aawyAB9rmqOaP8fadcCl4" in command
        assert "--ak" in command
        assert "client-id:client-secret" in command
        assert "--oo" in command

    def test_collect_audio_files(self, tmp_path: Path):
        (tmp_path / "a.mp3").write_bytes(b"a")
        (tmp_path / "nested").mkdir(parents=True)
        (tmp_path / "nested" / "b.flac").write_bytes(b"b")
        (tmp_path / "skip.txt").write_bytes(b"x")

        files = collect_audio_files(tmp_path)
        assert len(files) == 2

    def test_is_spotify_download_enabled_requires_binary(self, monkeypatch):
        settings = _settings()
        monkeypatch.setattr("bot.services.spotify_dl.shutil.which", lambda _: None)
        assert is_spotify_download_enabled(settings) is False

        monkeypatch.setattr("bot.services.spotify_dl.shutil.which", lambda _: "/usr/bin/spotifydl")
        assert is_spotify_download_enabled(settings) is True


class TestRunSpotifyDl:
    def test_raises_when_binary_missing(self, monkeypatch):
        monkeypatch.setattr("bot.services.spotify_dl.shutil.which", lambda _: None)
        with pytest.raises(SpotifyDlNotFoundError):
            run_spotify_dl(
                "https://open.spotify.com/track/abc",
                Path("/tmp/out"),
                _settings(),
            )

    def test_run_spotify_dl_success(self, monkeypatch, tmp_path: Path):
        monkeypatch.setattr("bot.services.spotify_dl.shutil.which", lambda _: "/usr/bin/spotifydl")

        def fake_run(command, **kwargs):
            output_dir = Path(command[command.index("--o") + 1])
            (output_dir / "Track One.mp3").write_bytes(b"audio")
            class Result:
                returncode = 0
                stdout = "Finished!"
                stderr = ""
            return Result()

        monkeypatch.setattr("bot.services.spotify_dl.subprocess.run", fake_run)

        tracks = run_spotify_dl(
            "https://open.spotify.com/album/4aawyAB9rmqOaP8fadcCl4",
            tmp_path,
            _settings(),
        )

        assert tracks == [SpotifyDlTrack(path=tmp_path / "Track One.mp3", title="Track One")]

    def test_run_spotify_dl_failure(self, monkeypatch, tmp_path: Path):
        monkeypatch.setattr("bot.services.spotify_dl.shutil.which", lambda _: "/usr/bin/spotifydl")

        def fake_run(command, **kwargs):
            class Result:
                returncode = 1
                stdout = ""
                stderr = "download failed"
            return Result()

        monkeypatch.setattr("bot.services.spotify_dl.subprocess.run", fake_run)

        with pytest.raises(SpotifyDlError, match="download failed"):
            run_spotify_dl(
                "https://open.spotify.com/track/abc",
                tmp_path,
                _settings(),
            )

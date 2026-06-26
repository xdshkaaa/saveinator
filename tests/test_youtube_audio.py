from pathlib import Path
from unittest.mock import MagicMock, patch

import pytest
import yt_dlp

from bot.config import Settings
from bot.services.spotify_models import NormalizedSpotifyTrack
from bot.services.youtube_audio import (
    YoutubeAudioError,
    YoutubeAudioNotFoundError,
    build_track_search_query,
    download_track_from_youtube,
    resolve_youtube_video_id,
    youtube_watch_url,
)


def _settings(**overrides) -> Settings:
    base = {
        "bot_token": "test-token",
        "spotify_dl_output_format": "mp3",
        "spotify_track_timeout_seconds": 30,
    }
    base.update(overrides)
    return Settings(**base)


def _track() -> NormalizedSpotifyTrack:
    return NormalizedSpotifyTrack(
        source_id="track-id",
        disc_number=1,
        track_number=1,
        title="Track One",
        artists="Artist One",
        duration_ms=180000,
        spotify_url="https://open.spotify.com/track/track-id",
    )


def test_build_track_search_query():
    assert build_track_search_query(_track()) == "Artist One - Track One"


def test_youtube_watch_url():
    assert youtube_watch_url("abc123") == "https://youtube.com/watch?v=abc123"


def test_resolve_youtube_video_id_returns_first_search_result():
    settings = _settings()
    fake_info = {"entries": [{"id": "video123"}]}

    with patch("bot.services.youtube_audio.yt_dlp.YoutubeDL") as ydl_cls:
        ydl = ydl_cls.return_value.__enter__.return_value
        ydl.extract_info.return_value = fake_info

        video_id = resolve_youtube_video_id("Artist One - Track One", settings)

    assert video_id == "video123"
    ydl.extract_info.assert_called_once_with("ytsearch1:Artist One - Track One", download=False)


def test_resolve_youtube_video_id_returns_none_on_failure():
    settings = _settings()

    with patch("bot.services.youtube_audio.yt_dlp.YoutubeDL") as ydl_cls:
        ydl = ydl_cls.return_value.__enter__.return_value
        ydl.extract_info.side_effect = yt_dlp.utils.DownloadError("search failed")

        assert resolve_youtube_video_id("missing track", settings) is None


def test_download_track_from_youtube_requires_url():
    settings = _settings()
    with pytest.raises(YoutubeAudioError, match="YouTube URL is required"):
        download_track_from_youtube(_track(), Path("/tmp/out"), settings, youtube_url="")


def test_download_track_from_youtube_uses_python_api(tmp_path: Path):
    settings = _settings()
    audio_file = tmp_path / "Track One.mp3"
    audio_file.write_bytes(b"audio")

    with patch("bot.services.youtube_audio.yt_dlp.YoutubeDL") as ydl_cls:
        ydl = ydl_cls.return_value.__enter__.return_value

        def fake_download(urls):
            assert urls == ["https://youtube.com/watch?v=video123"]

        ydl.download.side_effect = fake_download

        with patch("bot.services.youtube_audio._find_audio_file", return_value=audio_file):
            result = download_track_from_youtube(
                _track(),
                tmp_path,
                settings,
                youtube_url=youtube_watch_url("video123"),
            )

    assert result == audio_file
    opts = ydl_cls.call_args[0][0]
    assert opts["format"] == "bestaudio/best"
    assert opts["js_runtimes"] == {"deno": {}}
    assert opts["remote_components"] == {"ejs:github"}
    assert opts["postprocessors"][0]["preferredcodec"] == "mp3"


def test_download_track_from_youtube_raises_when_yt_dlp_missing(monkeypatch, tmp_path: Path):
    monkeypatch.setattr("bot.services.youtube_audio.is_yt_dlp_available", lambda: False)
    with pytest.raises(YoutubeAudioNotFoundError):
        download_track_from_youtube(
            _track(),
            tmp_path,
            _settings(),
            youtube_url=youtube_watch_url("video123"),
        )

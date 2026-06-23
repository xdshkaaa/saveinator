"""Tests for workers/downloader.py helpers (no yt-dlp dependency)."""

from pathlib import Path

import pytest

from workers.downloader import (
    XTargetReplyNoMediaError,
    download_with_reply_filter,
    _extract_status_id_from_url,
    _entry_matches_status_id,
    _entry_has_media,
    _find_entry_index,
)


class TestExtractStatusIdFromUrl:
    def test_x_domain(self):
        assert _extract_status_id_from_url("https://x.com/user/status/12345") == "12345"

    def test_twitter_domain(self):
        assert _extract_status_id_from_url("https://twitter.com/user/status/12345") == "12345"

    def test_with_query(self):
        assert _extract_status_id_from_url("https://x.com/user/status/12345?s=20") == "12345"

    def test_trailing_slash(self):
        assert _extract_status_id_from_url("https://x.com/user/status/12345/") == "12345"

    def test_no_match(self):
        assert _extract_status_id_from_url("https://example.com") is None


class TestEntryMatchesStatusId:
    def test_matches_id(self):
        assert _entry_matches_status_id({"id": "12345"}, "12345") is True

    def test_matches_display_id(self):
        assert _entry_matches_status_id({"display_id": "12345"}, "12345") is True

    def test_matches_webpage_url(self):
        assert (
            _entry_matches_status_id(
                {"webpage_url": "https://x.com/user/status/12345"}, "12345"
            )
            is True
        )

    def test_matches_original_url(self):
        assert (
            _entry_matches_status_id(
                {"original_url": "https://twitter.com/user/status/12345"}, "12345"
            )
            is True
        )

    def test_matches_url(self):
        assert (
            _entry_matches_status_id(
                {"url": "https://x.com/user/status/12345/video.mp4"}, "12345"
            )
            is True
        )

    def test_no_match(self):
        assert _entry_matches_status_id({"id": "99999"}, "12345") is False

    def test_empty_entry(self):
        assert _entry_matches_status_id({}, "12345") is False


class TestEntryHasMedia:
    def test_with_url(self):
        assert _entry_has_media({"url": "https://example.com/video.mp4"}) is True

    def test_with_formats_list(self):
        assert _entry_has_media({"formats": [{"url": "..."}]}) is True

    def test_no_media(self):
        assert _entry_has_media({"id": "12345", "title": "text only"}) is False

    def test_empty_dict(self):
        assert _entry_has_media({}) is False


class TestFindEntryIndex:
    def test_middle_entry(self):
        entries = [{"id": "1"}, {"id": "2"}, {"id": "3"}]
        assert _find_entry_index(entries, "2") == 2

    def test_not_found(self):
        entries = [{"id": "1"}, {"id": "2"}]
        assert _find_entry_index(entries, "999") is None

    def test_first_entry(self):
        entries = [{"id": "target"}, {"id": "other"}]
        assert _find_entry_index(entries, "target") == 1

    def test_last_entry(self):
        entries = [{"id": "a"}, {"id": "b"}, {"id": "c"}]
        assert _find_entry_index(entries, "c") == 3

    def test_empty_list(self):
        assert _find_entry_index([], "1") is None


class TestDownloadWithReplyFilter:
    def test_x_status_rejects_external_youtube_card(self, monkeypatch, tmp_path: Path):
        calls: list[tuple[str, bool]] = []

        class FakeYoutubeDL:
            def __init__(self, _opts):
                pass

            def __enter__(self):
                return self

            def __exit__(self, *_args):
                return False

            def extract_info(self, url, download):
                calls.append((url, download))
                return {
                    "id": "kdn3qYwzMDQ",
                    "extractor_key": "Youtube",
                    "title": "The Weirdest Create Mod Addon",
                    "original_url": "https://x.com/FrMinecraft/status/2069305477947695422",
                    "webpage_url": "https://www.youtube.com/watch?v=kdn3qYwzMDQ",
                    "formats": [{"url": "https://youtube.example/video.mp4"}],
                }

        monkeypatch.setattr("yt_dlp.YoutubeDL", FakeYoutubeDL)

        with pytest.raises(XTargetReplyNoMediaError):
            download_with_reply_filter(
                "https://x.com/FrMinecraft/status/2069305477947695422",
                tmp_path,
                "best",
                "2069305477947695422",
            )

        assert calls == [
            ("https://x.com/FrMinecraft/status/2069305477947695422", False),
        ]

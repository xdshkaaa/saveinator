import pytest
from bot.services.formats import extract_format_options


class TestFormats:
    def test_empty_formats(self):
        opts = extract_format_options([])
        assert opts == []

    def test_audio_only(self):
        formats = [
            {"format_id": "audio-1", "acodec": "mp4a", "vcodec": "none", "filesize": 5_000_000},
        ]
        opts = extract_format_options(formats)
        assert len(opts) == 1
        assert opts[0].is_audio_only

    def test_video_with_audio(self):
        formats = [
            {"format_id": "720p", "acodec": "mp4a", "vcodec": "avc1", "height": 720, "filesize": 50_000_000},
        ]
        opts = extract_format_options(formats)
        assert len(opts) == 1
        assert opts[0].height == 720
        assert not opts[0].is_audio_only

    def test_dedup_by_height(self):
        formats = [
            {"format_id": "22", "acodec": "mp4a", "vcodec": "avc1", "height": 720, "filesize": 50_000_000},
            {"format_id": "136", "acodec": "mp4a", "vcodec": "avc1", "height": 720, "filesize": 48_000_000},
            {"format_id": "137", "acodec": "mp4a", "vcodec": "avc1", "height": 1080, "filesize": 100_000_000},
        ]
        opts = extract_format_options(formats)
        heights = [o.height for o in opts if o.height]
        assert heights == [1080, 720]

    def test_sort_descending(self):
        formats = [
            {"format_id": "360p", "acodec": "mp4a", "vcodec": "avc1", "height": 360},
            {"format_id": "1080p", "acodec": "mp4a", "vcodec": "avc1", "height": 1080},
            {"format_id": "720p", "acodec": "mp4a", "vcodec": "avc1", "height": 720},
        ]
        opts = extract_format_options(formats)
        heights = [o.height for o in opts if o.height]
        assert heights == [1080, 720, 360]

    def test_fallback_best_combined(self):
        formats = [
            {"format_id": "video-only", "acodec": "none", "vcodec": "avc1", "height": 720},
            {"format_id": "audio-only", "acodec": "mp4a", "vcodec": "none"},
        ]
        opts = extract_format_options(formats)
        assert any(o.is_audio_only for o in opts)
        video_opts = [o for o in opts if not o.is_audio_only]
        assert len(video_opts) >= 1

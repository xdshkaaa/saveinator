from pathlib import Path

import pytest

from workers.video_processor import RATIO_DIMENSIONS, apply_aspect_ratio, target_dimensions


def test_target_dimensions_for_all_ratios():
    assert target_dimensions("16_9", 1080) == (1920, 1080)
    assert target_dimensions("21_9", 720) == (1680, 720)
    assert target_dimensions("9_16", 480) == (480, 854)


def test_ratio_dimensions_cover_all_qualities():
    for ratio_map in RATIO_DIMENSIONS.values():
        assert set(ratio_map) == {1080, 720, 480}


def test_apply_aspect_ratio_invokes_ffmpeg(monkeypatch, tmp_path: Path):
    source = tmp_path / "video.mp4"
    source.write_bytes(b"video")

    captured: dict[str, list[str]] = {}

    def fake_run(command, capture_output=True, text=True, check=False):
        captured["command"] = command
        output = Path(command[-1])
        output.write_bytes(b"processed")
        class Result:
            returncode = 0
            stdout = ""
            stderr = ""
        return Result()

    monkeypatch.setattr("workers.video_processor.subprocess.run", fake_run)

    result = apply_aspect_ratio(source, "16_9", 1080)

    assert result.name == "video_16_9.mp4"
    assert "-vf" in captured["command"]
    vf_index = captured["command"].index("-vf")
    assert "crop=1920:1080" in captured["command"][vf_index + 1]
    assert captured["command"][-1].endswith("video_16_9.mp4")

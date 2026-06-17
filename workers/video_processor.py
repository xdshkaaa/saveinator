import subprocess
from pathlib import Path

RATIO_DIMENSIONS: dict[str, dict[int, tuple[int, int]]] = {
    "16_9": {1080: (1920, 1080), 720: (1280, 720), 480: (854, 480)},
    "21_9": {1080: (2560, 1080), 720: (1680, 720), 480: (1120, 480)},
    "9_16": {1080: (1080, 1920), 720: (720, 1280), 480: (480, 854)},
}

_VIDEO_EXTENSIONS = frozenset({".mp4", ".webm", ".mkv", ".mov", ".m4v"})


class VideoProcessingError(Exception):
    pass


def target_dimensions(aspect_ratio: str, quality: int) -> tuple[int, int]:
    ratio_map = RATIO_DIMENSIONS.get(aspect_ratio)
    if not ratio_map:
        raise VideoProcessingError(f"unsupported aspect ratio: {aspect_ratio}")
    dimensions = ratio_map.get(quality)
    if dimensions is None:
        raise VideoProcessingError(f"unsupported quality for ratio: {quality}")
    return dimensions


def _run_ffmpeg(command: list[str]) -> None:
    result = subprocess.run(command, capture_output=True, text=True, check=False)
    if result.returncode != 0:
        detail = (result.stderr or result.stdout or f"exit code {result.returncode}").strip()
        raise VideoProcessingError(detail[:500])


def apply_aspect_ratio(source_path: Path, aspect_ratio: str, quality: int) -> Path:
    width, height = target_dimensions(aspect_ratio, quality)
    output_path = source_path.with_name(f"{source_path.stem}_{aspect_ratio}.mp4")

    vf = (
        f"scale={width}:{height}:force_original_aspect_ratio=increase,"
        f"crop={width}:{height}"
    )
    base_command = [
        "ffmpeg",
        "-y",
        "-i",
        str(source_path),
        "-vf",
        vf,
        "-c:v",
        "libx264",
        "-preset",
        "fast",
    ]

    try:
        _run_ffmpeg([*base_command, "-c:a", "copy", str(output_path)])
    except VideoProcessingError:
        _run_ffmpeg([*base_command, "-c:a", "aac", "-b:a", "128k", str(output_path)])

    return output_path

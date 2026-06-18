import subprocess
from pathlib import Path

RATIO_DIMENSIONS: dict[str, dict[int, tuple[int, int]]] = {
    "16_9": {1080: (1920, 1080), 720: (1280, 720), 480: (854, 480)},
    "21_9": {1080: (2560, 1080), 720: (1680, 720), 480: (1120, 480)},
    "9_16": {1080: (1080, 1920), 720: (720, 1280), 480: (480, 854)},
}

_FFMPEG_TIMEOUT_SECONDS = 300


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


def _probe_dimensions(source_path: Path) -> tuple[int, int] | None:
    result = subprocess.run(
        [
            "ffprobe",
            "-v",
            "error",
            "-select_streams",
            "v:0",
            "-show_entries",
            "stream=width,height",
            "-of",
            "csv=p=0:s=x",
            str(source_path),
        ],
        capture_output=True,
        text=True,
        check=False,
        timeout=30,
    )
    if result.returncode != 0:
        return None
    raw = (result.stdout or "").strip()
    if "x" not in raw:
        return None
    width_str, height_str = raw.split("x", 1)
    try:
        return int(width_str), int(height_str)
    except ValueError:
        return None


def _run_ffmpeg(command: list[str]) -> None:
    try:
        result = subprocess.run(
            command,
            capture_output=True,
            text=True,
            check=False,
            timeout=_FFMPEG_TIMEOUT_SECONDS,
        )
    except subprocess.TimeoutExpired as exc:
        raise VideoProcessingError(
            f"ffmpeg timed out after {_FFMPEG_TIMEOUT_SECONDS} seconds"
        ) from exc
    if result.returncode != 0:
        detail = (result.stderr or result.stdout or f"exit code {result.returncode}").strip()
        raise VideoProcessingError(detail[:500])


def apply_aspect_ratio(source_path: Path, aspect_ratio: str, quality: int) -> Path:
    width, height = target_dimensions(aspect_ratio, quality)
    output_path = source_path.with_name(f"{source_path.stem}_{aspect_ratio}.mp4")

    probed = _probe_dimensions(source_path)
    if probed == (width, height):
        if source_path == output_path:
            return source_path
        _run_ffmpeg([
            "ffmpeg",
            "-y",
            "-i",
            str(source_path),
            "-c",
            "copy",
            "-movflags",
            "+faststart",
            str(output_path),
        ])
        return output_path

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
        "ultrafast",
        "-crf",
        "28",
        "-threads",
        "1",
        "-movflags",
        "+faststart",
    ]

    try:
        _run_ffmpeg([*base_command, "-c:a", "copy", str(output_path)])
    except VideoProcessingError:
        _run_ffmpeg([*base_command, "-c:a", "aac", "-b:a", "128k", str(output_path)])

    return output_path

import asyncio
import os
from pathlib import Path

import structlog

logger = structlog.get_logger()


async def remux_merge(
    video_path: Path, audio_path: Path, output_path: Path
) -> bool:
    proc = await asyncio.create_subprocess_exec(
        "ffmpeg", "-y",
        "-i", str(video_path),
        "-i", str(audio_path),
        "-c:v", "copy",
        "-c:a", "aac",
        "-movflags", "+faststart",
        str(output_path),
        stdout=asyncio.subprocess.DEVNULL,
        stderr=asyncio.subprocess.PIPE,
    )
    try:
        _, stderr = await asyncio.wait_for(proc.communicate(), timeout=300)
    except asyncio.TimeoutError:
        proc.kill()
        await proc.wait()
        return False

    if proc.returncode != 0:
        logger.error("ffmpeg remux failed", stderr=stderr.decode()[:500])
        return False

    return output_path.exists() and os.path.getsize(output_path) > 0


async def compress_video(
    input_path: Path, output_path: Path, target_mb: float = 45
) -> bool:
    proc = await asyncio.create_subprocess_exec(
        "ffmpeg", "-y",
        "-i", str(input_path),
        "-c:v", "libx264",
        "-crf", "28",
        "-preset", "fast",
        "-c:a", "aac",
        "-b:a", "128k",
        "-movflags", "+faststart",
        str(output_path),
        stdout=asyncio.subprocess.DEVNULL,
        stderr=asyncio.subprocess.PIPE,
    )
    try:
        _, stderr = await asyncio.wait_for(proc.communicate(), timeout=300)
    except asyncio.TimeoutError:
        proc.kill()
        await proc.wait()
        return False

    if proc.returncode != 0:
        logger.error("ffmpeg compress failed", stderr=stderr.decode()[:500])
        return False

    return output_path.exists() and os.path.getsize(output_path) > 0

QUALITY_HEIGHTS: tuple[int, ...] = (1080, 720, 480)


def build_youtube_format(target_height: int, aspect_ratio: str | None = None) -> str:
    # Prefer muxed streams to avoid a heavy ffmpeg merge on small VPS hosts.
    # For 9:16 the quality label refers to width (e.g. 1080p → 1080×1920).
    limit_dim = "width" if aspect_ratio == "9_16" else "height"
    return (
        f"best[{limit_dim}<={target_height}][vcodec!=none][acodec!=none]/"
        f"best[{limit_dim}<={target_height}][ext=mp4]/"
        f"best[{limit_dim}<={target_height}]/"
        f"bestvideo[{limit_dim}<={target_height}]+bestaudio/"
        f"bestvideo+bestaudio/best"
    )

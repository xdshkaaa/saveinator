QUALITY_HEIGHTS: tuple[int, ...] = (1080, 720, 480)


def build_youtube_format(target_height: int) -> str:
    # Prefer muxed streams to avoid a heavy ffmpeg merge on small VPS hosts.
    return (
        f"best[height<={target_height}][vcodec!=none][acodec!=none]/"
        f"best[height<={target_height}][ext=mp4]/"
        f"best[height<={target_height}]/"
        f"bestvideo[height<={target_height}]+bestaudio/"
        f"bestvideo+bestaudio/best"
    )

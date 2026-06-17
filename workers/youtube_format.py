QUALITY_HEIGHTS: tuple[int, ...] = (1080, 720, 480)


def build_youtube_format(target_height: int) -> str:
    return (
        f"bestvideo[height<={target_height}][ext=mp4]+bestaudio[ext=m4a]/"
        f"bestvideo[height<={target_height}]+bestaudio/"
        f"best[height<={target_height}]/best"
    )

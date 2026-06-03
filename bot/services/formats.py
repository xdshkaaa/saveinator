from dataclasses import dataclass


@dataclass
class FormatOption:
    format_id: str
    height: int | None
    label: str
    size_bytes: int | None
    is_audio_only: bool


def extract_format_options(formats: list[dict]) -> list[FormatOption]:
    seen_heights: set[int] = set()
    options: list[FormatOption] = []
    has_audio = False

    for fmt in formats:
        height = fmt.get("height")
        acodec = fmt.get("acodec", "")
        vcodec = fmt.get("vcodec", "")

        if vcodec and vcodec != "none" and acodec and acodec != "none":
            if height and height not in seen_heights:
                seen_heights.add(height)
                filesize = fmt.get("filesize") or fmt.get("filesize_approx")
                options.append(FormatOption(
                    format_id=fmt["format_id"],
                    height=height,
                    label=f"{height}p",
                    size_bytes=filesize,
                    is_audio_only=False,
                ))

        if not vcodec or vcodec == "none":
            if acodec and acodec != "none":
                has_audio = True

    options.sort(key=lambda o: o.height or 0, reverse=True)

    if not options:
        best = _find_best_combined(formats)
        if best:
            options.append(FormatOption(
                format_id=best["format_id"],
                height=best.get("height"),
                label="best" if not best.get("height") else f"{best['height']}p",
                size_bytes=best.get("filesize") or best.get("filesize_approx"),
                is_audio_only=False,
            ))

    if not options:
        best = _find_video_only(formats)
        if best:
            options.append(FormatOption(
                format_id=best["format_id"],
                height=best.get("height"),
                label="best" if not best.get("height") else f"{best['height']}p",
                size_bytes=best.get("filesize") or best.get("filesize_approx"),
                is_audio_only=False,
            ))

    if has_audio:
        options.append(FormatOption(
            format_id="bestaudio",
            height=None,
            label="audio",
            size_bytes=None,
            is_audio_only=True,
        ))

    return options


def _find_best_combined(formats: list[dict]) -> dict | None:
    for fmt in formats:
        vcodec = fmt.get("vcodec", "")
        acodec = fmt.get("acodec", "")
        if vcodec and vcodec != "none" and acodec and acodec != "none":
            return fmt
    return None


def _find_video_only(formats: list[dict]) -> dict | None:
    for fmt in formats:
        vcodec = fmt.get("vcodec", "")
        if vcodec and vcodec != "none":
            return fmt
    return None

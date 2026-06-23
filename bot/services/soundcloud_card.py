from aiogram.types import InlineKeyboardButton, InlineKeyboardMarkup

from bot.config import Settings
from bot.locale import get
from bot.services.soundcloud_audio import is_soundcloud_download_enabled
from bot.services.soundcloud_models import NormalizedSoundCloudRelease


def _format_duration(duration_ms: int) -> str:
    total_seconds = max(duration_ms // 1000, 0)
    minutes, seconds = divmod(total_seconds, 60)
    return f"{minutes}:{seconds:02d}"


def build_soundcloud_card_text(
    release: NormalizedSoundCloudRelease,
    lang: str,
    settings: Settings,
) -> str:
    is_single_track = release.release_type == "track" or len(release.tracks) == 1

    if is_single_track and release.tracks:
        track = release.tracks[0]
        lines = [
            get(
                "soundcloud.track_card_title",
                lang,
                artist=track.artist or release.artist,
                title=track.title or release.title,
            ),
            get("soundcloud.duration", lang, duration=_format_duration(track.duration_ms)),
        ]
        if track.genre:
            lines.append(get("soundcloud.genre", lang, genre=track.genre))
    else:
        lines = [
            get(
                "soundcloud.card_title",
                lang,
                artist=release.artist,
                name=release.title,
            ),
            get("soundcloud.tracks_count", lang, count=len(release.tracks)),
        ]

    lines.append(get("soundcloud.source", lang))

    if not is_soundcloud_download_enabled(settings):
        lines.append("")
        lines.append(get("soundcloud.no_download", lang))

    return "\n".join(lines)


def build_soundcloud_keyboard(release: NormalizedSoundCloudRelease, lang: str) -> InlineKeyboardMarkup:
    return InlineKeyboardMarkup(
        inline_keyboard=[
            [
                InlineKeyboardButton(
                    text=get("soundcloud.btn_open", lang),
                    url=release.soundcloud_url,
                )
            ]
        ]
    )

from aiogram.types import InlineKeyboardButton, InlineKeyboardMarkup

from bot.config import Settings
from bot.locale import get
from bot.services.spotify_dl import is_spotify_download_enabled
from bot.services.spotify_models import NormalizedSpotifyRelease


def _album_type_label(album_type: str, lang: str) -> str:
    key = f"spotify.type_{album_type}"
    label = get(key, lang)
    if label == key:
        return album_type
    return label


def _format_duration(duration_ms: int) -> str:
    total_seconds = max(duration_ms // 1000, 0)
    minutes, seconds = divmod(total_seconds, 60)
    return f"{minutes}:{seconds:02d}"


def build_spotify_card_text(
    release: NormalizedSpotifyRelease,
    lang: str,
    settings: Settings,
) -> str:
    is_single_track = release.album_type == "track" or (
        len(release.tracks) == 1 and release.album_type not in ("album", "single", "compilation")
    )

    if is_single_track and release.tracks:
        track = release.tracks[0]
        lines = [
            get(
                "spotify.track_card_title",
                lang,
                artist=track.artists,
                title=track.title,
            ),
            get("spotify.duration", lang, duration=_format_duration(track.duration_ms)),
        ]
    else:
        lines = [
            get(
                "spotify.card_title",
                lang,
                artist=release.artists,
                name=release.title,
            ),
            get(
                "spotify.album_type",
                lang,
                type=_album_type_label(release.album_type, lang),
            ),
            get("spotify.tracks_count", lang, count=len(release.tracks)),
            get("spotify.release_date", lang, date=release.release_date),
        ]

    if is_spotify_download_enabled(settings):
        lines.append("")
        lines.append(get("spotify.download_queued", lang))
    else:
        lines.append("")
        lines.append(get("spotify.no_download", lang))

    return "\n".join(lines)


def build_spotify_keyboard(release: NormalizedSpotifyRelease, lang: str) -> InlineKeyboardMarkup:
    return InlineKeyboardMarkup(
        inline_keyboard=[
            [
                InlineKeyboardButton(
                    text=get("spotify.btn_open", lang),
                    url=release.spotify_url,
                )
            ]
        ]
    )

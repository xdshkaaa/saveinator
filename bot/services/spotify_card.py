from aiogram.types import InlineKeyboardButton, InlineKeyboardMarkup

from bot.locale import get
from bot.services.audio_providers import get_legal_audio_provider
from bot.services.spotify_models import SpotifyAlbum


def _album_type_label(album_type: str, lang: str) -> str:
    key = f"spotify.type_{album_type}"
    label = get(key, lang)
    if label == key:
        return album_type
    return label


def build_spotify_card_text(album: SpotifyAlbum, lang: str) -> str:
    lines = [
        get("spotify.card_title", lang, name=album.album_name),
        get("spotify.artist", lang, artist=album.artists),
        get(
            "spotify.album_type",
            lang,
            type=_album_type_label(album.album_type, lang),
        ),
        get("spotify.tracks_count", lang, count=len(album.tracks)),
        get("spotify.release_date", lang, date=album.release_date),
    ]

    if get_legal_audio_provider() is None:
        lines.append("")
        lines.append(get("spotify.no_download", lang))

    return "\n".join(lines)


def build_spotify_keyboard(album: SpotifyAlbum, lang: str) -> InlineKeyboardMarkup:
    return InlineKeyboardMarkup(
        inline_keyboard=[
            [
                InlineKeyboardButton(
                    text=get("spotify.btn_open", lang),
                    url=album.spotify_url,
                )
            ]
        ]
    )

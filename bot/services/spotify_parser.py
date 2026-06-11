import re
from dataclasses import dataclass
from typing import Literal

SpotifyLinkType = Literal["album", "track"]

_SPOTIFY_ID = r"[A-Za-z0-9]{22}"

_ALBUM_URL = re.compile(
    rf"(?:https?://)?(?:open\.)?spotify\.com/album/({_SPOTIFY_ID})",
    re.IGNORECASE,
)
_TRACK_URL = re.compile(
    rf"(?:https?://)?(?:open\.)?spotify\.com/track/({_SPOTIFY_ID})",
    re.IGNORECASE,
)
_ALBUM_URI = re.compile(rf"spotify:album:({_SPOTIFY_ID})", re.IGNORECASE)
_TRACK_URI = re.compile(rf"spotify:track:({_SPOTIFY_ID})", re.IGNORECASE)


@dataclass(frozen=True)
class SpotifyLink:
    type: SpotifyLinkType
    id: str


def is_valid_spotify_id(spotify_id: str) -> bool:
    return bool(re.fullmatch(_SPOTIFY_ID, spotify_id))


def parse_spotify_link(url_or_uri: str) -> SpotifyLink | None:
    text = url_or_uri.strip()
    if "?" in text:
        text = text.split("?", 1)[0]
    if "#" in text:
        text = text.split("#", 1)[0]

    for pattern, link_type in (
        (_ALBUM_URI, "album"),
        (_TRACK_URI, "track"),
        (_ALBUM_URL, "album"),
        (_TRACK_URL, "track"),
    ):
        match = pattern.search(text)
        if match:
            spotify_id = match.group(1)
            if is_valid_spotify_id(spotify_id):
                return SpotifyLink(type=link_type, id=spotify_id)
    return None

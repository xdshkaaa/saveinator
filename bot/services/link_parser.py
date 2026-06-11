import re
from dataclasses import dataclass
from db.models import Platform

_SPOTIFY_ALBUM_ID = r"[A-Za-z0-9]{22}"

_PATTERNS: list[tuple[Platform, re.Pattern[str]]] = [
    (
        Platform.YOUTUBE,
        re.compile(
            r"(?:https?://)?(?:www\.)?(?:youtube\.com/watch\?v=|youtu\.be/|youtube\.com/shorts/)"
            r"[\w-]{11}(?:[?&]\S*)?",
            re.IGNORECASE,
        ),
    ),
    (
        Platform.TIKTOK,
        re.compile(
            r"(?:https?://)?(?:www\.)?(?:"
            r"tiktok\.com/@[\w.-]+/video/\d+(?:[/?#&]\S*)?"
            r"|(?:vm|vt)\.tiktok\.com/[\w-]+/?(?:[?&]\S*)?"
            r")",
            re.IGNORECASE,
        ),
    ),
    (
        Platform.INSTAGRAM,
        re.compile(
            r"(?:https?://)?(?:www\.)?"
            r"instagram\.com/(?:p|reels?|tv)/[\w-]+/?(?:[?&]\S*)?",
            re.IGNORECASE,
        ),
    ),
    (
        Platform.X,
        re.compile(
            r"(?:https?://)?(?:www\.)?"
            r"(?:x|twitter)\.com/[\w-]+/status/\d+/?(?:[?&]\S*)?",
            re.IGNORECASE,
        ),
    ),
    (
        Platform.SPOTIFY,
        re.compile(
            r"(?:https?://)?(?:open\.)?spotify\.com/album/"
            + _SPOTIFY_ALBUM_ID
            + r"(?:[/?#&]\S*)?",
            re.IGNORECASE,
        ),
    ),
]

_URL_EXTRACTOR = re.compile(r"https?://\S+", re.IGNORECASE)
_SPOTIFY_URI_EXTRACTOR = re.compile(
    rf"spotify:album:({_SPOTIFY_ALBUM_ID})",
    re.IGNORECASE,
)


@dataclass
class ParsedLink:
    platform: Platform
    url: str
    spotify_album_id: str | None = None


def extract_spotify_album_id(url_or_uri: str) -> str | None:
    uri_match = _SPOTIFY_URI_EXTRACTOR.search(url_or_uri)
    if uri_match:
        return uri_match.group(1)

    for platform, pattern in _PATTERNS:
        if platform != Platform.SPOTIFY:
            continue
        match = pattern.search(url_or_uri)
        if match:
            id_match = re.search(_SPOTIFY_ALBUM_ID, match.group(0))
            if id_match:
                return id_match.group(0)
    return None


def _parse_spotify_uri(text: str, seen: set[str]) -> list[ParsedLink]:
    results: list[ParsedLink] = []
    for match in _SPOTIFY_URI_EXTRACTOR.finditer(text):
        album_id = match.group(1)
        if album_id in seen:
            continue
        seen.add(album_id)
        results.append(
            ParsedLink(
                platform=Platform.SPOTIFY,
                url=match.group(0),
                spotify_album_id=album_id,
            )
        )
    return results


def extract_urls(text: str) -> list[ParsedLink]:
    results: list[ParsedLink] = []
    seen_spotify_ids: set[str] = set()
    raw_urls = _URL_EXTRACTOR.findall(text)

    for raw_url in raw_urls:
        url = raw_url.rstrip(".,;:!?)]}")

        for platform, pattern in _PATTERNS:
            match = pattern.search(url)
            if match:
                matched_url = match.group(0)
                spotify_album_id = None
                if platform == Platform.SPOTIFY:
                    spotify_album_id = extract_spotify_album_id(matched_url)
                    if spotify_album_id:
                        seen_spotify_ids.add(spotify_album_id)
                results.append(
                    ParsedLink(
                        platform=platform,
                        url=matched_url,
                        spotify_album_id=spotify_album_id,
                    )
                )
                break
        else:
            results.append(ParsedLink(platform=Platform.UNKNOWN, url=url))

    results.extend(_parse_spotify_uri(text, seen_spotify_ids))
    return results

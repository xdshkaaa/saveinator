import re
from dataclasses import dataclass

from db.models import Platform
from bot.services.spotify_parser import SpotifyLink, parse_spotify_link
from bot.services.soundcloud_parser import SoundCloudLink, parse_soundcloud_link

_SPOTIFY_ID_IN_URL = r"[A-Za-z0-9]{22}"

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
    *[
        (
            Platform.INSTAGRAM,
            re.compile(
                r"(?:https?://)?(?:www\.)?" + pattern,
                re.IGNORECASE,
            ),
        )
        for pattern in (
            r"instagram\.com/(?:p|reels?|tv)/[\w-]+/?(?:[?&#]\S*)?",
            r"instagr\.am/(?:p|reels?|tv)/[\w-]+/?(?:[?&#]\S*)?",
            r"instagram\.com/share/(?:reel|p)/[\w-]+/?(?:[?&#]\S*)?",
            r"instagram\.com/stories/[\w.-]+/\d+/?(?:[?&#]\S*)?",
        )
    ],
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
            r"(?:https?://)?(?:open\.)?spotify\.com/(?:album|track)/"
            + _SPOTIFY_ID_IN_URL
            + r"(?:[/?#&]\S*)?",
            re.IGNORECASE,
        ),
    ),
    (
        Platform.SOUNDCLOUD,
        re.compile(
            r"(?:https?://)?(?:www\.)?soundcloud\.com/[\w.-]+/sets/[\w.-]+(?:[/?#&]\S*)?",
            re.IGNORECASE,
        ),
    ),
    (
        Platform.SOUNDCLOUD,
        re.compile(
            r"(?:https?://)?on\.soundcloud\.com/[\w-]+(?:[/?#&]\S*)?",
            re.IGNORECASE,
        ),
    ),
    (
        Platform.SOUNDCLOUD,
        re.compile(
            r"(?:https?://)?(?:www\.)?soundcloud\.com/[\w.-]+/(?!sets/)[\w.-]+(?:[/?#&]\S*)?",
            re.IGNORECASE,
        ),
    ),
    (
        Platform.PINTEREST,
        re.compile(
            r"(?:https?://)?(?:www\.)?pinterest\.com/pin/[\w-]+/?(?:[?&]\S*)?",
            re.IGNORECASE,
        ),
    ),
    (
        Platform.PINTEREST,
        re.compile(
            r"(?:https?://)?pin\.it/[\w-]+/?(?:[?&]\S*)?",
            re.IGNORECASE,
        ),
    ),
    (
        Platform.PINTEREST,
        re.compile(
            r"(?:https?://)?(?:www\.)?pinterest\.com/[\w-]+/[\w-]+/?(?:[?&]\S*)?",
            re.IGNORECASE,
        ),
    ),
]

_URL_EXTRACTOR = re.compile(r"https?://\S+", re.IGNORECASE)
_SPOTIFY_URI_EXTRACTOR = re.compile(
    rf"spotify:(?:album|track):({_SPOTIFY_ID_IN_URL})",
    re.IGNORECASE,
)


@dataclass
class ParsedLink:
    platform: Platform
    url: str
    spotify_link: SpotifyLink | None = None
    soundcloud_link: SoundCloudLink | None = None


def extract_spotify_link(url_or_uri: str) -> SpotifyLink | None:
    return parse_spotify_link(url_or_uri)


def extract_soundcloud_link(url: str) -> SoundCloudLink | None:
    return parse_soundcloud_link(url)


def _parse_spotify_uri(text: str, seen: set[str]) -> list[ParsedLink]:
    results: list[ParsedLink] = []
    for match in _SPOTIFY_URI_EXTRACTOR.finditer(text):
        spotify_link = parse_spotify_link(match.group(0))
        if not spotify_link or spotify_link.id in seen:
            continue
        seen.add(spotify_link.id)
        results.append(
            ParsedLink(
                platform=Platform.SPOTIFY,
                url=match.group(0),
                spotify_link=spotify_link,
            )
        )
    return results


def extract_urls(text: str) -> list[ParsedLink]:
    results: list[ParsedLink] = []
    seen_spotify_ids: set[str] = set()
    seen_soundcloud_urls: set[str] = set()
    raw_urls = _URL_EXTRACTOR.findall(text)

    for raw_url in raw_urls:
        url = raw_url.rstrip(".,;:!?)]}")

        for platform, pattern in _PATTERNS:
            match = pattern.search(url)
            if match:
                matched_url = match.group(0)
                spotify_link = None
                soundcloud_link = None
                if platform == Platform.SPOTIFY:
                    spotify_link = parse_spotify_link(matched_url)
                    if spotify_link:
                        seen_spotify_ids.add(spotify_link.id)
                elif platform == Platform.SOUNDCLOUD:
                    soundcloud_link = parse_soundcloud_link(matched_url)
                    if soundcloud_link:
                        if soundcloud_link.url in seen_soundcloud_urls:
                            break
                        seen_soundcloud_urls.add(soundcloud_link.url)
                results.append(
                    ParsedLink(
                        platform=platform,
                        url=matched_url,
                        spotify_link=spotify_link,
                        soundcloud_link=soundcloud_link,
                    )
                )
                break
        else:
            results.append(ParsedLink(platform=Platform.UNKNOWN, url=url))

    results.extend(_parse_spotify_uri(text, seen_spotify_ids))
    return results

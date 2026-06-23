import re
from dataclasses import dataclass
from typing import Literal
from urllib.parse import urlparse

SoundCloudLinkType = Literal["track", "playlist", "short"]

_DISCOVER_PLAYLIST_URL = re.compile(
    r"(?:https?://)?(?:www\.)?soundcloud\.com/discover/sets/[^\s?#]+",
    re.IGNORECASE,
)
_PLAYLIST_URL = re.compile(
    r"(?:https?://)?(?:www\.)?soundcloud\.com/[\w.-]+/sets/[^\s?#]+",
    re.IGNORECASE,
)
_TRACK_URL = re.compile(
    r"(?:https?://)?(?:www\.)?soundcloud\.com/[\w.-]+/(?!sets/)[\w.-]+",
    re.IGNORECASE,
)
_SHORT_URL = re.compile(
    r"(?:https?://)?on\.soundcloud\.com/[\w-]+",
    re.IGNORECASE,
)


@dataclass(frozen=True)
class SoundCloudLink:
    type: SoundCloudLinkType
    url: str


def _strip_query_and_fragment(url: str) -> str:
    text = url.strip()
    if "?" in text:
        text = text.split("?", 1)[0]
    if "#" in text:
        text = text.split("#", 1)[0]
    return text.rstrip("/")


def _normalize_url(url: str) -> str:
    cleaned = _strip_query_and_fragment(url)
    parsed = urlparse(cleaned if "://" in cleaned else f"https://{cleaned}")
    scheme = parsed.scheme or "https"
    netloc = parsed.netloc.lower()
    path = parsed.path.rstrip("/")
    return f"{scheme}://{netloc}{path}"


def parse_soundcloud_link(url: str) -> SoundCloudLink | None:
    text = url.strip()
    if not text:
        return None

    for pattern, link_type in (
        (_DISCOVER_PLAYLIST_URL, "playlist"),
        (_PLAYLIST_URL, "playlist"),
        (_SHORT_URL, "short"),
        (_TRACK_URL, "track"),
    ):
        match = pattern.search(text)
        if match:
            matched_url = match.group(0)
            return SoundCloudLink(type=link_type, url=_normalize_url(matched_url))
    return None

import re
from dataclasses import dataclass
from enum import Enum


class TikTokPostType(str, Enum):
    VIDEO = "video"
    PHOTO = "photo"
    UNKNOWN = "unknown"


@dataclass(frozen=True)
class TikTokLink:
    url: str
    post_id: str
    post_type: TikTokPostType
    resolved_url: str | None = None


_TIKTOK_SHORT_RE = re.compile(
    r"https?://(?:vm|vt)\.tiktok\.com/([\w-]+)",
    re.IGNORECASE,
)

_TIKTOK_FULL_RE = re.compile(
    r"https?://(?:www\.)?tiktok\.com/@[\w.-]*/(video|photo)/(\d+)",
    re.IGNORECASE,
)

_TIKTOK_T_SHORT_RE = re.compile(
    r"https?://(?:www\.)?tiktok\.com/t/([\w-]+)",
    re.IGNORECASE,
)

_TIKTOK_MOBILE_RE = re.compile(
    r"https?://m\.tiktok\.com/v/(\d+)\.html",
    re.IGNORECASE,
)

_TIKTOK_NON_CONTENT_PATHS = frozenset({
    "foryou", "following", "explore", "live", "discover", "search",
})


def is_tiktok_non_content_url(url: str) -> bool:
    """True for TikTok homepage/feed links that do not point at a specific post."""
    from urllib.parse import urlparse

    parsed = urlparse(url.strip())
    host = (parsed.hostname or "").lower()
    if host.startswith("www."):
        host = host[4:]
    if host not in ("tiktok.com", "m.tiktok.com"):
        return False

    path = parsed.path.strip("/")
    if not path:
        return True

    first_segment = path.split("/", 1)[0].lower()
    return first_segment in _TIKTOK_NON_CONTENT_PATHS


def parse_tiktok_url(url: str) -> TikTokLink | None:
    """Parse a TikTok URL into a structured link with post type detection."""
    short_match = _TIKTOK_SHORT_RE.search(url)
    if short_match:
        return TikTokLink(
            url=short_match.group(0),
            post_id=short_match.group(1),
            post_type=TikTokPostType.UNKNOWN,
        )

    full_match = _TIKTOK_FULL_RE.search(url)
    if full_match:
        type_str = full_match.group(1).lower()
        post_type = TikTokPostType(type_str)
        post_id = full_match.group(2)
        return TikTokLink(
            url=full_match.group(0),
            post_id=post_id,
            post_type=post_type,
        )

    return None


def is_tiktok_url(url: str) -> bool:
    """Check if a URL is a TikTok URL."""
    from bot.services.link_parser import extract_urls
    from db.models import Platform

    links = extract_urls(url)
    return any(link.platform == Platform.TIKTOK for link in links)

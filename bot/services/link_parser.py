import re
from dataclasses import dataclass
from db.models import Platform

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
            r"(?:https?://)?(?:www\.)?(?:tiktok\.com/@[\w.-]+/video/\d+|vm\.tiktok\.com/\w+)",
            re.IGNORECASE,
        ),
    ),
    (
        Platform.INSTAGRAM,
        re.compile(
            r"(?:https?://)?(?:www\.)?(?:instagram\.com/(?:p|reel|tv)/[\w-]+)",
            re.IGNORECASE,
        ),
    ),
]

_URL_EXTRACTOR = re.compile(r"https?://\S+", re.IGNORECASE)


@dataclass
class ParsedLink:
    platform: Platform
    url: str


def extract_urls(text: str) -> list[ParsedLink]:
    results: list[ParsedLink] = []
    raw_urls = _URL_EXTRACTOR.findall(text)

    for raw_url in raw_urls:
        url = raw_url.rstrip(".,;:!?)]}")

        for platform, pattern in _PATTERNS:
            match = pattern.search(url)
            if match:
                results.append(ParsedLink(platform=platform, url=match.group(0)))
                break
        else:
            results.append(ParsedLink(platform=Platform.UNKNOWN, url=url))

    return results

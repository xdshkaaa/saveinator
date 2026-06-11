import re

from bot.services.pinterest_models import ParsedPinterestUrl, PinterestUrlType

_PIN_PATTERN = re.compile(
    r"(?:https?://)?(?:www\.)?pinterest\.com/pin/[\w-]+/?(?:[?&]\S*)?",
    re.IGNORECASE,
)
_SHORT_PATTERN = re.compile(
    r"(?:https?://)?pin\.it/[\w-]+/?(?:[?&]\S*)?",
    re.IGNORECASE,
)
_BOARD_PATTERN = re.compile(
    r"(?:https?://)?(?:www\.)?pinterest\.com/[\w-]+/[\w-]+/?(?:[?&]\S*)?",
    re.IGNORECASE,
)
_RESERVED_BOARD_SEGMENTS = frozenset(
    {"pin", "search", "ideas", "today", "shopping", "videos", "topics"}
)


def parse_pinterest_url(url: str) -> ParsedPinterestUrl | None:
    """Return parsed Pinterest URL metadata, or None if not a Pinterest URL."""
    normalized = url.strip()
    if not normalized:
        return None

    if match := _PIN_PATTERN.search(normalized):
        return ParsedPinterestUrl(url=match.group(0), url_type=PinterestUrlType.PIN)

    if match := _SHORT_PATTERN.search(normalized):
        return ParsedPinterestUrl(url=match.group(0), url_type=PinterestUrlType.SHORT)

    if match := _BOARD_PATTERN.search(normalized):
        path = match.group(0).split("pinterest.com/", 1)[-1]
        first_segment = path.split("/", 1)[0].lower()
        if first_segment not in _RESERVED_BOARD_SEGMENTS:
            return ParsedPinterestUrl(url=match.group(0), url_type=PinterestUrlType.BOARD)

    return None


def is_valid_pinterest_url(url: str) -> bool:
    return parse_pinterest_url(url) is not None

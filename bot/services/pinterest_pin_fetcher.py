import re
from typing import Any

import requests
from pinterest_dl.api.api import Api
from pinterest_dl.domain.cookies import CookieJar
from pinterest_dl.domain.media import PinterestMedia
from pinterest_dl.download import request_builder
from pinterest_dl.exceptions import EmptyResponseError
from pinterest_dl.parsers.response import ResponseParser

_PIN_RESOURCE = "https://www.pinterest.com/resource/PinResource/get/"
_PIN_ID_PATTERN = re.compile(r"pin/(\d+)")


def extract_pin_id(url: str) -> str | None:
    match = _PIN_ID_PATTERN.search(url)
    return match.group(1) if match else None


def resolve_pin_url(url: str, session: requests.Session, timeout: float) -> str:
    if "pin.it/" not in url.lower():
        return url
    response = session.get(url, allow_redirects=True, timeout=timeout)
    return response.url


def _session_from_api_client(dl_client: Any, timeout: float) -> requests.Session:
    session = requests.Session()
    session.headers.update({"User-Agent": Api.USER_AGENT})
    session.headers.update({"x-pinterest-pws-handler": "www/pin/[id].js"})
    if getattr(dl_client, "cookies", None):
        jar = dl_client.cookies
        if isinstance(jar, (CookieJar, requests.cookies.RequestsCookieJar)):
            session.cookies.update(jar)
        elif isinstance(jar, dict):
            session.cookies.update(jar)
    session.get("https://www.pinterest.com/", timeout=timeout)
    return session


def _enrich_pin_caption(pin_data: dict[str, Any], media: PinterestMedia) -> None:
    if media.alt and media.alt.strip():
        return
    caption = pin_data.get("grid_title") or pin_data.get("description") or ""
    if caption:
        media.alt = str(caption)


def fetch_pin_media(url: str, dl_client: Any, timeout: float) -> list[PinterestMedia]:
    """Fetch the main media for a single Pinterest pin (not related pins)."""
    session = _session_from_api_client(dl_client, timeout)
    resolved_url = resolve_pin_url(url, session, timeout)
    pin_id = extract_pin_id(resolved_url)
    if not pin_id:
        raise ValueError(f"Could not resolve Pinterest pin id from URL: {url}")

    request_url = request_builder.build_get(
        _PIN_RESOURCE,
        {"id": pin_id, "field_set_key": "detailed"},
        f"/pin/{pin_id}/",
    )
    response = session.get(request_url, timeout=timeout)
    response.raise_for_status()

    pin_data = response.json()["resource_response"]["data"]
    if not isinstance(pin_data, dict):
        raise EmptyResponseError("PinResource returned no pin data")

    media_items = ResponseParser.from_responses([pin_data], (0, 0))
    if not media_items:
        raise EmptyResponseError("PinResource returned no downloadable media")

    for item in media_items:
        item.origin = f"https://www.pinterest.com/pin/{pin_id}/"
        _enrich_pin_caption(pin_data, item)

    return media_items[:1]

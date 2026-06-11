"""Optional external audio provider hooks for custom deployments.

Spotify audio downloads use yt-dlp YouTube search via bot.services.youtube_audio.
"""

from dataclasses import dataclass
from pathlib import Path
from typing import Protocol


@dataclass
class SearchResult:
    query: str
    source_url: str
    title: str


@dataclass
class DownloadedFile:
    path: Path
    title: str


class AudioSearchProvider(Protocol):
    async def search_track(self, artist: str, title: str) -> SearchResult | None:
        ...


class AudioDownloadProvider(Protocol):
    async def download(self, result: SearchResult, output_dir: str) -> DownloadedFile:
        ...

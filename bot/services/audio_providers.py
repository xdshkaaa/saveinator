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


_search_provider: AudioSearchProvider | None = None
_download_provider: AudioDownloadProvider | None = None


def register_audio_providers(
    search_provider: AudioSearchProvider | None,
    download_provider: AudioDownloadProvider | None,
) -> None:
    global _search_provider, _download_provider
    _search_provider = search_provider
    _download_provider = download_provider


def get_search_provider() -> AudioSearchProvider | None:
    return _search_provider


def get_download_provider() -> AudioDownloadProvider | None:
    return _download_provider


def has_audio_download_pipeline() -> bool:
    return _search_provider is not None and _download_provider is not None

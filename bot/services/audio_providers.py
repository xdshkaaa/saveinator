from typing import Protocol


class AudioProvider(Protocol):
    async def download_track(self, spotify_url: str, output_dir: str) -> str:
        ...


def get_legal_audio_provider() -> AudioProvider | None:
    return None

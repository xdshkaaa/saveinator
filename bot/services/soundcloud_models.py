from dataclasses import asdict, dataclass
from typing import Literal

SoundCloudReleaseType = Literal["track", "playlist"]


@dataclass
class NormalizedSoundCloudTrack:
    source_id: str
    title: str
    artist: str
    duration_ms: int
    soundcloud_url: str
    artwork_url: str | None
    genre: str
    description: str
    created_at: str
    track_number: int


@dataclass
class NormalizedSoundCloudRelease:
    source_id: str
    title: str
    artist: str
    release_type: SoundCloudReleaseType
    artwork_url: str | None
    soundcloud_url: str
    tracks: list[NormalizedSoundCloudTrack]


def track_to_dict(track: NormalizedSoundCloudTrack) -> dict:
    return asdict(track)


def release_to_dict(release: NormalizedSoundCloudRelease) -> dict:
    return {
        "source_id": release.source_id,
        "title": release.title,
        "artist": release.artist,
        "release_type": release.release_type,
        "artwork_url": release.artwork_url,
        "soundcloud_url": release.soundcloud_url,
        "tracks": [track_to_dict(track) for track in release.tracks],
    }


def release_from_dict(data: dict) -> NormalizedSoundCloudRelease:
    tracks = [
        NormalizedSoundCloudTrack(**track_data)
        for track_data in data.get("tracks", [])
    ]
    return NormalizedSoundCloudRelease(
        source_id=data["source_id"],
        title=data["title"],
        artist=data["artist"],
        release_type=data["release_type"],
        artwork_url=data.get("artwork_url"),
        soundcloud_url=data["soundcloud_url"],
        tracks=tracks,
    )

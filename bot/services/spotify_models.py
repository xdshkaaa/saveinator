from dataclasses import dataclass


@dataclass
class NormalizedSpotifyTrack:
    source_id: str
    title: str
    artists: str
    duration_ms: int
    spotify_url: str
    disc_number: int
    track_number: int


@dataclass
class NormalizedSpotifyRelease:
    source_id: str
    title: str
    artists: str
    album_type: str
    release_date: str
    cover_url: str | None
    spotify_url: str
    tracks: list[NormalizedSpotifyTrack]


# Backward-compatible aliases used in existing imports/tests.
SpotifyTrack = NormalizedSpotifyTrack
SpotifyAlbum = NormalizedSpotifyRelease


def _format_artists(artists: list[dict]) -> str:
    return ", ".join(artist.get("name", "") for artist in artists if artist.get("name"))


def normalize_track(api_track: dict) -> NormalizedSpotifyTrack:
    track_id = api_track.get("id", "")
    return NormalizedSpotifyTrack(
        source_id=track_id,
        title=api_track.get("name", ""),
        artists=_format_artists(api_track.get("artists") or []),
        duration_ms=api_track.get("duration_ms", 0),
        spotify_url=api_track.get("external_urls", {}).get(
            "spotify", f"https://open.spotify.com/track/{track_id}"
        ),
        disc_number=api_track.get("disc_number", 1),
        track_number=api_track.get("track_number", 0),
    )


def normalize_album(api_album: dict, paginated_tracks: list[dict]) -> NormalizedSpotifyRelease:
    images = api_album.get("images") or []
    cover_url = images[0].get("url") if images else None

    tracks: list[NormalizedSpotifyTrack] = []
    for item in paginated_tracks:
        tracks.append(
            NormalizedSpotifyTrack(
                source_id=item.get("id", ""),
                title=item.get("name", ""),
                artists=_format_artists(item.get("artists") or []),
                duration_ms=item.get("duration_ms", 0),
                spotify_url=item.get("external_urls", {}).get("spotify", ""),
                disc_number=item.get("disc_number", 1),
                track_number=item.get("track_number", 0),
            )
        )

    album_id = api_album.get("id", "")
    return NormalizedSpotifyRelease(
        source_id=album_id,
        title=api_album.get("name", ""),
        album_type=api_album.get("album_type", "album"),
        artists=_format_artists(api_album.get("artists") or []),
        release_date=api_album.get("release_date", ""),
        cover_url=cover_url,
        spotify_url=api_album.get("external_urls", {}).get(
            "spotify", f"https://open.spotify.com/album/{album_id}"
        ),
        tracks=tracks,
    )


def release_from_track(api_track: dict) -> NormalizedSpotifyRelease:
    """Build a single-track release card from a track API response."""
    track = normalize_track(api_track)
    album = api_track.get("album") or {}
    images = album.get("images") or []
    cover_url = images[0].get("url") if images else None
    album_id = album.get("id", track.source_id)

    return NormalizedSpotifyRelease(
        source_id=album_id or track.source_id,
        title=track.title,
        album_type="track",
        artists=track.artists,
        release_date=album.get("release_date", ""),
        cover_url=cover_url,
        spotify_url=track.spotify_url,
        tracks=[track],
    )

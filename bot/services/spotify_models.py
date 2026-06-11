from dataclasses import dataclass


@dataclass
class SpotifyTrack:
    disc_number: int
    track_number: int
    title: str
    artists: str
    duration_ms: int
    spotify_url: str


@dataclass
class SpotifyAlbum:
    album_id: str
    album_name: str
    album_type: str
    artists: str
    release_date: str
    cover_url: str | None
    spotify_url: str
    tracks: list[SpotifyTrack]


def _format_artists(artists: list[dict]) -> str:
    return ", ".join(artist.get("name", "") for artist in artists if artist.get("name"))


def normalize_album(api_album: dict, paginated_tracks: list[dict]) -> SpotifyAlbum:
    images = api_album.get("images") or []
    cover_url = images[0].get("url") if images else None

    tracks: list[SpotifyTrack] = []
    for item in paginated_tracks:
        tracks.append(
            SpotifyTrack(
                disc_number=item.get("disc_number", 1),
                track_number=item.get("track_number", 0),
                title=item.get("name", ""),
                artists=_format_artists(item.get("artists") or []),
                duration_ms=item.get("duration_ms", 0),
                spotify_url=item.get("external_urls", {}).get("spotify", ""),
            )
        )

    album_id = api_album.get("id", "")
    return SpotifyAlbum(
        album_id=album_id,
        album_name=api_album.get("name", ""),
        album_type=api_album.get("album_type", "album"),
        artists=_format_artists(api_album.get("artists") or []),
        release_date=api_album.get("release_date", ""),
        cover_url=cover_url,
        spotify_url=api_album.get("external_urls", {}).get(
            "spotify", f"https://open.spotify.com/album/{album_id}"
        ),
        tracks=tracks,
    )

from unittest.mock import AsyncMock

import pytest
from sqlalchemy import select

from bot.services.release_metadata_store import (
    persist_soundcloud_release,
    persist_spotify_release,
)
from bot.services.soundcloud_models import NormalizedSoundCloudRelease, NormalizedSoundCloudTrack
from bot.services.spotify_models import NormalizedSpotifyRelease, NormalizedSpotifyTrack
from db.models import MusicReleaseMetadata, Platform


def _spotify_release() -> NormalizedSpotifyRelease:
    return NormalizedSpotifyRelease(
        source_id="album-1",
        title="Test Album",
        album_type="album",
        artists="Artist One",
        release_date="2021-01-01",
        cover_url="https://example.com/cover.jpg",
        spotify_url="https://open.spotify.com/album/album-1",
        tracks=[
            NormalizedSpotifyTrack(
                source_id="track-1",
                title="Track One",
                artists="Artist One",
                duration_ms=180000,
                spotify_url="https://open.spotify.com/track/track-1",
                disc_number=1,
                track_number=1,
            )
        ],
    )


def _soundcloud_release() -> NormalizedSoundCloudRelease:
    return NormalizedSoundCloudRelease(
        source_id="sc-1",
        title="Test Track",
        artist="SC Artist",
        release_type="track",
        artwork_url="https://example.com/art.jpg",
        soundcloud_url="https://soundcloud.com/artist/test-track",
        tracks=[
            NormalizedSoundCloudTrack(
                source_id="sc-track-1",
                title="Test Track",
                artist="SC Artist",
                duration_ms=200000,
                soundcloud_url="https://soundcloud.com/artist/test-track",
                artwork_url="https://example.com/art.jpg",
                genre="Electronic",
                description="desc",
                created_at="20240101",
                track_number=1,
            )
        ],
    )


@pytest.mark.asyncio
async def test_persist_spotify_release_creates_row(db_session):
    release = _spotify_release()

    await persist_spotify_release(release)

    row = (
        await db_session.execute(
            select(MusicReleaseMetadata).where(
                MusicReleaseMetadata.platform == Platform.SPOTIFY,
                MusicReleaseMetadata.source_id == "album-1",
            )
        )
    ).scalar_one()

    assert row.title == "Test Album"
    assert row.artist == "Artist One"
    assert row.track_count == 1
    assert row.payload["title"] == "Test Album"
    assert row.payload["tracks"][0]["title"] == "Track One"
    assert row.first_fetched_at is not None
    assert row.last_fetched_at is not None


@pytest.mark.asyncio
async def test_persist_spotify_release_updates_existing_row(db_session):
    release = _spotify_release()
    await persist_spotify_release(release)

    updated = _spotify_release()
    updated.title = "Updated Album"
    updated.tracks.append(
        NormalizedSpotifyTrack(
            source_id="track-2",
            title="Track Two",
            artists="Artist One",
            duration_ms=200000,
            spotify_url="https://open.spotify.com/track/track-2",
            disc_number=1,
            track_number=2,
        )
    )

    await persist_spotify_release(updated)

    rows = (
        await db_session.execute(
            select(MusicReleaseMetadata).where(
                MusicReleaseMetadata.platform == Platform.SPOTIFY,
                MusicReleaseMetadata.source_id == "album-1",
            )
        )
    ).scalars().all()

    assert len(rows) == 1
    assert rows[0].title == "Updated Album"
    assert rows[0].track_count == 2
    assert len(rows[0].payload["tracks"]) == 2


@pytest.mark.asyncio
async def test_persist_soundcloud_release_creates_row(db_session):
    release = _soundcloud_release()

    await persist_soundcloud_release(release)

    row = (
        await db_session.execute(
            select(MusicReleaseMetadata).where(
                MusicReleaseMetadata.platform == Platform.SOUNDCLOUD,
                MusicReleaseMetadata.source_id == "sc-1",
            )
        )
    ).scalar_one()

    assert row.title == "Test Track"
    assert row.artist == "SC Artist"
    assert row.release_type == "track"
    assert row.payload["soundcloud_url"] == "https://soundcloud.com/artist/test-track"


@pytest.mark.asyncio
async def test_persist_spotify_release_swallows_db_errors(monkeypatch):
    monkeypatch.setattr(
        "bot.services.release_metadata_store._upsert_release",
        AsyncMock(side_effect=RuntimeError("db down")),
    )

    await persist_spotify_release(_spotify_release())

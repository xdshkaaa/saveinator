import structlog
from sqlalchemy import select
from sqlalchemy.dialects.postgresql import insert

from bot.services.soundcloud_models import (
    NormalizedSoundCloudRelease,
    release_to_dict as soundcloud_release_to_dict,
)
from bot.services.spotify_models import (
    NormalizedSpotifyRelease,
    release_to_dict as spotify_release_to_dict,
)
from db.models import MusicReleaseMetadata, Platform, utc_now_naive
from db.session import async_session_factory

logger = structlog.get_logger()


async def _upsert_release(
    platform: Platform,
    source_id: str,
    release_type: str,
    canonical_url: str,
    title: str,
    artist: str,
    track_count: int,
    payload: dict,
) -> None:
    now = utc_now_naive()
    values = {
        "platform": platform,
        "source_id": source_id,
        "release_type": release_type,
        "canonical_url": canonical_url,
        "title": title,
        "artist": artist,
        "track_count": track_count,
        "payload": payload,
        "first_fetched_at": now,
        "last_fetched_at": now,
    }
    update_values = {
        "release_type": release_type,
        "canonical_url": canonical_url,
        "title": title,
        "artist": artist,
        "track_count": track_count,
        "payload": payload,
        "last_fetched_at": now,
    }

    async with async_session_factory() as session:
        dialect_name = session.get_bind().dialect.name
        if dialect_name == "postgresql":
            stmt = insert(MusicReleaseMetadata).values(**values)
            stmt = stmt.on_conflict_do_update(
                constraint="uq_music_release_platform_source",
                set_=update_values,
            )
            await session.execute(stmt)
        else:
            existing = (
                await session.execute(
                    select(MusicReleaseMetadata).where(
                        MusicReleaseMetadata.platform == platform,
                        MusicReleaseMetadata.source_id == source_id,
                    )
                )
            ).scalar_one_or_none()
            if existing is None:
                session.add(MusicReleaseMetadata(**values))
            else:
                for key, value in update_values.items():
                    setattr(existing, key, value)
        await session.commit()


async def persist_spotify_release(release: NormalizedSpotifyRelease) -> None:
    try:
        await _upsert_release(
            platform=Platform.SPOTIFY,
            source_id=release.source_id,
            release_type=release.album_type,
            canonical_url=release.spotify_url,
            title=release.title,
            artist=release.artists,
            track_count=len(release.tracks),
            payload=spotify_release_to_dict(release),
        )
    except Exception:
        logger.exception(
            "spotify metadata persist failed",
            source_id=release.source_id,
            exc_info=True,
        )


async def persist_soundcloud_release(release: NormalizedSoundCloudRelease) -> None:
    try:
        await _upsert_release(
            platform=Platform.SOUNDCLOUD,
            source_id=release.source_id,
            release_type=release.release_type,
            canonical_url=release.soundcloud_url,
            title=release.title,
            artist=release.artist,
            track_count=len(release.tracks),
            payload=soundcloud_release_to_dict(release),
        )
    except Exception:
        logger.exception(
            "soundcloud metadata persist failed",
            source_id=release.source_id,
            exc_info=True,
        )

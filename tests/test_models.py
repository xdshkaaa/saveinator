import pytest

from db.models import (
    BannedLink,
    Chat,
    Download,
    DownloadStatus,
    Language,
    MusicReleaseMetadata,
    Platform,
    User,
    utc_now_naive,
)
from workers.tasks import utc_now_naive as task_utc_now_naive


@pytest.mark.asyncio
async def test_timestamp_defaults_are_offset_naive(db_session):
    user = User(id=1, username="tester", language=Language.EN)
    chat = Chat(id=1, title="test", type="private")
    download = Download(
        user_id=1,
        chat_id=1,
        url="https://youtu.be/dQw4w9WgXcQ",
        platform=Platform.YOUTUBE,
        format_id="best",
        status=DownloadStatus.COMPLETED,
        completed_at=task_utc_now_naive(),
    )
    banned_link = BannedLink(url_hash="abc123")

    db_session.add_all([user, chat, download, banned_link])
    await db_session.flush()

    values = [utc_now_naive(), user.created_at, chat.created_at, download.created_at, download.completed_at, banned_link.created_at]

    assert all(value.tzinfo is None for value in values)


@pytest.mark.asyncio
async def test_music_release_metadata_model(db_session):
    row = MusicReleaseMetadata(
        platform=Platform.SPOTIFY,
        source_id="album-smoke",
        release_type="album",
        canonical_url="https://open.spotify.com/album/album-smoke",
        title="Smoke Album",
        artist="Smoke Artist",
        track_count=1,
        payload={"title": "Smoke Album", "tracks": []},
    )
    db_session.add(row)
    await db_session.flush()

    assert row.id is not None
    assert row.first_fetched_at.tzinfo is None
    assert row.last_fetched_at.tzinfo is None

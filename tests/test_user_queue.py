import pytest

from bot.services.user_queue import (
    UserScenario,
    acquire_user_lock,
    extend_user_lock,
    lock_ttl_seconds,
    release_user_lock,
    release_user_lock_sync,
)


@pytest.fixture
async def fake_redis(monkeypatch):
    import fakeredis.aioredis

    server = fakeredis.FakeServer()
    async_client = fakeredis.aioredis.FakeRedis(server=server, decode_responses=True)

    import fakeredis

    sync_client = fakeredis.FakeRedis(server=server, decode_responses=True)

    async def _async_redis():
        return async_client

    monkeypatch.setattr("bot.services.redis_client._async_redis", async_client)
    monkeypatch.setattr("bot.services.redis_client.get_async_redis", _async_redis)
    monkeypatch.setattr("bot.services.redis_client._sync_redis", sync_client)
    monkeypatch.setattr("bot.services.redis_client.get_sync_redis", lambda: sync_client)
    return async_client


async def test_acquire_blocks_second_scenario(fake_redis):
    first = await acquire_user_lock(42, UserScenario.VIDEO)
    second = await acquire_user_lock(42, UserScenario.PINTEREST)

    assert first is not None
    assert second is None


async def test_release_allows_next_scenario(fake_redis):
    token = await acquire_user_lock(42, UserScenario.VIDEO)
    assert token is not None

    await release_user_lock(42, token, UserScenario.VIDEO)

    next_token = await acquire_user_lock(42, UserScenario.SPOTIFY, track_count=3)
    assert next_token is not None


async def test_extend_updates_ttl_for_spotify(fake_redis):
    token = await acquire_user_lock(42, UserScenario.SPOTIFY, track_count=1)
    assert token is not None

    extended = await extend_user_lock(42, token, UserScenario.SPOTIFY, track_count=5)
    assert extended is True

    ttl = await fake_redis.ttl("user_busy:42")
    assert ttl >= lock_ttl_seconds(UserScenario.SPOTIFY, track_count=5) - 2


def test_release_sync_clears_lock(fake_redis):
    from bot.services.user_queue import acquire_user_lock_sync

    token = acquire_user_lock_sync(99, UserScenario.VIDEO)
    assert token is not None

    release_user_lock_sync(99, token, UserScenario.VIDEO)

    next_token = acquire_user_lock_sync(99, UserScenario.PINTEREST)
    assert next_token is not None

import pytest

from bot.services.user_bans import ban_user, is_user_banned, list_banned_users, unban_user


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


async def test_ban_and_unban_user(fake_redis):
    await ban_user(12345)
    assert await is_user_banned(12345)
    assert await list_banned_users() == [12345]

    removed = await unban_user(12345)
    assert removed is True
    assert not await is_user_banned(12345)
    assert await list_banned_users() == []


async def test_unban_missing_user_returns_false(fake_redis):
    assert await unban_user(99999) is False

import pytest

from bot.services.runtime_settings import (
    get_runtime_int,
    platform_download_timeout_seconds,
    platform_max_file_mb,
    reset_runtime,
    set_runtime_int,
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


async def test_runtime_override_applies_for_youtube(fake_redis):
    await reset_runtime(None)
    await set_runtime_int("youtube.max_file_mb", 1500)
    await set_runtime_int("youtube.timeout_sec", 180)

    assert platform_max_file_mb("youtube") == 1500
    assert platform_download_timeout_seconds("youtube") == 180


async def test_runtime_reset_restores_defaults(fake_redis):
    await set_runtime_int("tiktok.max_file_mb", 99)
    assert platform_max_file_mb("tiktok") == 99

    await reset_runtime("tiktok.max_file_mb")
    assert platform_max_file_mb("tiktok") == get_runtime_int(
        "tiktok.max_file_mb",
        default=50,
    )


async def test_runtime_read_failure_falls_back_to_default(monkeypatch):
    def broken_redis():
        raise ConnectionError("redis down")

    monkeypatch.setattr("bot.services.runtime_settings.get_sync_redis", broken_redis)
    assert platform_max_file_mb("youtube") >= 1

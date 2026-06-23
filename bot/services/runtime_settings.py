from dataclasses import dataclass

import structlog

from bot.config import Settings, settings
from bot.metrics import record_rpc_failure
from bot.services.redis_client import get_async_redis, get_sync_redis

logger = structlog.get_logger()

REDIS_KEY = "saveinator:runtime_settings"


@dataclass(frozen=True)
class RuntimeSettingDef:
    redis_key: str
    settings_attr: str
    service: str
    kind: str  # "max_file_mb" | "timeout_sec"


SERVICE_ORDER = ("youtube", "tiktok", "instagram", "x", "spotify", "soundcloud", "pinterest")

RUNTIME_SETTINGS: tuple[RuntimeSettingDef, ...] = (
    RuntimeSettingDef("youtube.max_file_mb", "youtube_max_file_size_mb", "youtube", "max_file_mb"),
    RuntimeSettingDef("youtube.timeout_sec", "youtube_download_timeout_seconds", "youtube", "timeout_sec"),
    RuntimeSettingDef("tiktok.max_file_mb", "send_video_limit_mb", "tiktok", "max_file_mb"),
    RuntimeSettingDef("tiktok.timeout_sec", "download_timeout_seconds", "tiktok", "timeout_sec"),
    RuntimeSettingDef("instagram.max_file_mb", "send_video_limit_mb", "instagram", "max_file_mb"),
    RuntimeSettingDef("instagram.timeout_sec", "download_timeout_seconds", "instagram", "timeout_sec"),
    RuntimeSettingDef("x.max_file_mb", "send_video_limit_mb", "x", "max_file_mb"),
    RuntimeSettingDef("x.timeout_sec", "download_timeout_seconds", "x", "timeout_sec"),
    RuntimeSettingDef("spotify.timeout_sec", "spotify_track_timeout_seconds", "spotify", "timeout_sec"),
    RuntimeSettingDef("spotify.max_file_mb", "send_document_limit_mb", "spotify", "max_file_mb"),
    RuntimeSettingDef(
        "soundcloud.timeout_sec",
        "soundcloud_track_timeout_seconds",
        "soundcloud",
        "timeout_sec",
    ),
    RuntimeSettingDef("soundcloud.max_file_mb", "soundcloud_max_file_mb", "soundcloud", "max_file_mb"),
    RuntimeSettingDef("pinterest.max_file_mb", "send_video_limit_mb", "pinterest", "max_file_mb"),
    RuntimeSettingDef("pinterest.timeout_sec", "pinterest_timeout_seconds", "pinterest", "timeout_sec"),
    RuntimeSettingDef("global.document_limit_mb", "send_document_limit_mb", "global", "max_file_mb"),
    RuntimeSettingDef("global.telegram_upload_limit_mb", "telegram_bot_upload_limit_mb", "global", "max_file_mb"),
)

_SETTINGS_BY_REDIS_KEY = {item.redis_key: item for item in RUNTIME_SETTINGS}
_SETTINGS_BY_SERVICE: dict[str, list[RuntimeSettingDef]] = {}
for _item in RUNTIME_SETTINGS:
    _SETTINGS_BY_SERVICE.setdefault(_item.service, []).append(_item)


def _default_value(defn: RuntimeSettingDef, settings_obj: Settings | None = None) -> int:
    source = settings_obj or settings
    return int(getattr(source, defn.settings_attr))


def _parse_override(raw: str | None) -> int | None:
    if raw is None:
        return None
    try:
        return int(raw)
    except (TypeError, ValueError):
        logger.warning("invalid runtime setting override", raw=raw)
        return None


def get_runtime_int(redis_key: str, default: int | None = None) -> int:
    defn = _SETTINGS_BY_REDIS_KEY.get(redis_key)
    fallback = default if default is not None else (_default_value(defn) if defn else 0)
    try:
        redis_client = get_sync_redis()
        override = _parse_override(redis_client.hget(REDIS_KEY, redis_key))
        return override if override is not None else fallback
    except Exception:
        record_rpc_failure("redis")
        logger.warning("runtime settings read failed", key=redis_key, exc_info=True)
        return fallback


async def get_runtime_int_async(redis_key: str, default: int | None = None) -> int:
    defn = _SETTINGS_BY_REDIS_KEY.get(redis_key)
    fallback = default if default is not None else (_default_value(defn) if defn else 0)
    try:
        redis_client = await get_async_redis()
        override = _parse_override(await redis_client.hget(REDIS_KEY, redis_key))
        return override if override is not None else fallback
    except Exception:
        record_rpc_failure("redis")
        logger.warning("runtime settings read failed", key=redis_key, exc_info=True)
        return fallback


async def set_runtime_int(redis_key: str, value: int) -> None:
    if redis_key not in _SETTINGS_BY_REDIS_KEY:
        raise KeyError(redis_key)
    redis_client = await get_async_redis()
    await redis_client.hset(REDIS_KEY, redis_key, str(int(value)))


async def reset_runtime(redis_key: str | None = None) -> None:
    redis_client = await get_async_redis()
    if redis_key is None:
        await redis_client.delete(REDIS_KEY)
        return
    await redis_client.hdel(REDIS_KEY, redis_key)


def service_settings(service: str) -> list[RuntimeSettingDef]:
    return list(_SETTINGS_BY_SERVICE.get(service, []))


def setting_definition(redis_key: str) -> RuntimeSettingDef | None:
    return _SETTINGS_BY_REDIS_KEY.get(redis_key)


async def current_value(defn: RuntimeSettingDef) -> int:
    return await get_runtime_int_async(defn.redis_key, _default_value(defn))


async def all_current_values() -> dict[str, int]:
    values: dict[str, int] = {}
    for defn in RUNTIME_SETTINGS:
        values[defn.redis_key] = await current_value(defn)
    return values


def platform_max_file_mb(platform: str) -> int:
    return get_runtime_int(f"{platform}.max_file_mb")


def platform_download_timeout_seconds(platform: str) -> int:
    return get_runtime_int(f"{platform}.timeout_sec")


def send_document_limit_mb() -> int:
    return get_runtime_int("global.document_limit_mb")


def telegram_bot_upload_limit_mb() -> int:
    return get_runtime_int("global.telegram_upload_limit_mb")


def spotify_track_timeout_seconds() -> int:
    return get_runtime_int("spotify.timeout_sec")


def soundcloud_track_timeout_seconds() -> int:
    return get_runtime_int("soundcloud.timeout_sec")


def soundcloud_max_file_mb() -> int:
    return get_runtime_int("soundcloud.max_file_mb")


def pinterest_timeout_seconds() -> int:
    return get_runtime_int("pinterest.timeout_sec")


def pinterest_max_file_mb() -> int:
    return get_runtime_int("pinterest.max_file_mb")

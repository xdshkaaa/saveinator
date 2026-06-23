"""Runtime settings system — Redis-backed hot-swappable admin overrides.

Every setting has a typed definition (``RuntimeSettingDef``) in the
``RUNTIME_SETTINGS`` registry.  Admin changes go directly to a Redis
hash (``saveinator:runtime_settings``) and take effect on the *next*
read — there is no caching layer.

Fallback chain
--------------
1. Redis override (if key exists in the hash)
2. :class:`Settings` attribute from ``.env`` / env vars (the ``default``
   param / ``_default_value``)
3. Hardcoded fallback (the ``default`` param passed to a read function)
"""

from dataclasses import dataclass, field
from typing import Any, Callable, Literal

import structlog

from bot.config import Settings, settings
from bot.metrics import record_rpc_failure
from bot.services.redis_client import get_async_redis, get_sync_redis

logger = structlog.get_logger()

REDIS_KEY = "saveinator:runtime_settings"

# ---------------------------------------------------------------------------
# Types
# ---------------------------------------------------------------------------

SettingType = Literal["int", "bool", "enum", "list"]


@dataclass(frozen=True)
class RuntimeSettingDef:
    """Descriptor for a single runtime-overridable setting."""

    # -- identity -----------------------------------------------------------
    redis_key: str  # e.g. "youtube.max_file_mb"
    settings_attr: str  # attribute on the Settings class
    service: str  # "youtube" | "tiktok" | … | "global"
    kind: str  # "max_file_mb" | "timeout_sec" | "enabled" | …

    # -- type & validation --------------------------------------------------
    value_type: SettingType = "int"
    min_value: int | None = None
    max_value: int | None = None
    allowed_values: tuple[str, ...] = ()  # for "enum" / "list"
    validator: Callable[[Any], str | None] | None = None  # returns error msg or None

    # -- display ------------------------------------------------------------
    label_en: str = ""
    label_ru: str = ""
    desc_en: str = ""
    desc_ru: str = ""
    unit: str = ""  # e.g. "MB", "sec"

    # -- admin hints --------------------------------------------------------
    confirm_on_change: bool = False  # require confirmation before applying
    sensitive: bool = False  # never expose in admin UI (tokens, cookies)


# ---------------------------------------------------------------------------
# Display helpers
# ---------------------------------------------------------------------------

SERVICE_LABELS: dict[str, tuple[str, str]] = {
    "youtube": ("YouTube", "YouTube"),
    "tiktok": ("TikTok", "TikTok"),
    "instagram": ("Instagram", "Instagram"),
    "x": ("X / Twitter", "X / Twitter"),
    "spotify": ("Spotify", "Spotify"),
    "soundcloud": ("SoundCloud", "SoundCloud"),
    "pinterest": ("Pinterest", "Pinterest"),
    "global": ("Global", "Глобально"),
}

SERVICE_ORDER = ("youtube", "tiktok", "instagram", "x", "spotify", "soundcloud", "pinterest")


def service_label(service: str, lang: str) -> str:
    labels = SERVICE_LABELS.get(service, (service, service))
    return labels[1] if lang == "ru" else labels[0]


def kind_label(defn: RuntimeSettingDef, lang: str) -> str:
    return defn.label_ru if lang == "ru" else defn.label_en


def kind_description(defn: RuntimeSettingDef, lang: str) -> str:
    return defn.desc_ru if lang == "ru" else defn.desc_en


# ---------------------------------------------------------------------------
# Registry — all runtime settings
# ---------------------------------------------------------------------------

RUNTIME_SETTINGS: tuple[RuntimeSettingDef, ...] = (
    # -- global -------------------------------------------------------------
    RuntimeSettingDef(
        "global.document_limit_mb",
        "send_document_limit_mb",
        "global",
        "document_limit_mb",
        value_type="int", min_value=1, max_value=1999,
        label_en="Document limit", label_ru="Лимит документов", unit="MB",
        desc_en="Max file size for sending as a document.",
        desc_ru="Максимальный размер файла для отправки документом.",
    ),
    RuntimeSettingDef(
        "global.telegram_upload_limit_mb",
        "telegram_bot_upload_limit_mb",
        "global",
        "telegram_upload_limit_mb",
        value_type="int", min_value=1, max_value=1999,
        label_en="Telegram upload limit", label_ru="Лимит загрузки Telegram", unit="MB",
        desc_en="Actual upload limit via Telegram Bot API.",
        desc_ru="Фактический лимит загрузки через Telegram Bot API.",
    ),
    RuntimeSettingDef(
        "global.default_timeout_sec",
        "download_timeout_seconds",
        "global",
        "default_timeout_sec",
        value_type="int", min_value=10, max_value=3600,
        label_en="Default timeout", label_ru="Таймаут по умолчанию", unit="sec",
        desc_en="Default download timeout for platforms without a separate setting.",
        desc_ru="Дефолтный таймаут для платформ без отдельной настройки.",
    ),
    RuntimeSettingDef(
        "global.broadcast_delay_ms",
        "broadcast_delay_ms",
        "global",
        "broadcast_delay_ms",
        value_type="int", min_value=20, max_value=5000,
        label_en="Broadcast delay", label_ru="Задержка рассылки", unit="ms",
        desc_en="Pause between messages when broadcasting (milliseconds).",
        desc_ru="Пауза между сообщениями при рассылке (миллисекунды).",
    ),
    RuntimeSettingDef(
        "global.broadcast_batch_size",
        "broadcast_batch_size",
        "global",
        "broadcast_batch_size",
        value_type="int", min_value=1, max_value=100,
        label_en="Broadcast batch size", label_ru="Размер пачки рассылки",
        desc_en="Number of users to process in one batch.",
        desc_ru="Количество пользователей в одной пачке рассылки.",
    ),

    # -- youtube ------------------------------------------------------------
    RuntimeSettingDef(
        "youtube.max_file_mb",
        "youtube_max_file_size_mb",
        "youtube",
        "max_file_mb",
        value_type="int", min_value=1, max_value=1999,
        label_en="Max file", label_ru="Лимит файла", unit="MB",
        desc_en="Maximum size of the final video file.",
        desc_ru="Максимальный размер итогового видеофайла.",
    ),
    RuntimeSettingDef(
        "youtube.timeout_sec",
        "youtube_download_timeout_seconds",
        "youtube",
        "timeout_sec",
        value_type="int", min_value=30, max_value=3600,
        label_en="Timeout", label_ru="Таймаут", unit="sec",
        desc_en="Total download/processing timeout.",
        desc_ru="Общий таймаут скачивания/обработки.",
    ),
    RuntimeSettingDef(
        "youtube.allowed_qualities",
        "youtube_allowed_qualities",
        "youtube",
        "allowed_qualities",
        value_type="list", allowed_values=("1080", "720", "480"),
        label_en="Allowed qualities", label_ru="Доступные качества",
        desc_en="Comma-separated: 1080, 720, 480",
        desc_ru="Через запятую: 1080, 720, 480",
    ),
    RuntimeSettingDef(
        "youtube.default_quality",
        "youtube_default_quality",
        "youtube",
        "default_quality",
        value_type="enum", allowed_values=("1080", "720", "480", "ask"),
        label_en="Default quality", label_ru="Качество по умолчанию",
        desc_en="Default quality when user doesn't choose.",
        desc_ru="Качество по умолчанию, если пользователь не выбрал.",
    ),
    RuntimeSettingDef(
        "youtube.allowed_ratios",
        "youtube_allowed_ratios",
        "youtube",
        "allowed_ratios",
        value_type="list", allowed_values=("16_9", "21_9", "9_16"),
        label_en="Allowed ratios", label_ru="Доступные соотношения",
        desc_en="Comma-separated: 16_9, 21_9, 9_16",
        desc_ru="Через запятую: 16_9, 21_9, 9_16",
    ),
    RuntimeSettingDef(
        "youtube.transcode_enabled",
        "youtube_transcode_enabled",
        "youtube",
        "transcode_enabled",
        value_type="bool",
        label_en="Transcode enabled", label_ru="Транскодинг",
        desc_en="Allow aspect ratio transcoding.",
        desc_ru="Разрешить обработку соотношения сторон.",
    ),
    RuntimeSettingDef(
        "youtube.max_duration_sec",
        "youtube_max_duration_sec",
        "youtube",
        "max_duration_sec",
        value_type="int", min_value=0, max_value=86400,
        label_en="Max duration", label_ru="Макс. длительность", unit="sec",
        desc_en="Optional video duration limit (0 = no limit).",
        desc_ru="Лимит длительности видео (0 = без лимита).",
    ),

    # -- tiktok -------------------------------------------------------------
    RuntimeSettingDef(
        "tiktok.max_file_mb",
        "send_video_limit_mb",
        "tiktok",
        "max_file_mb",
        value_type="int", min_value=1, max_value=500,
        label_en="Max file", label_ru="Лимит файла", unit="MB",
    ),
    RuntimeSettingDef(
        "tiktok.timeout_sec",
        "download_timeout_seconds",
        "tiktok",
        "timeout_sec",
        value_type="int", min_value=10, max_value=300,
        label_en="Timeout", label_ru="Таймаут", unit="sec",
    ),
    RuntimeSettingDef(
        "tiktok.max_duration_sec",
        "tiktok_max_duration_sec",
        "tiktok",
        "max_duration_sec",
        value_type="int", min_value=0, max_value=600,
        label_en="Max duration", label_ru="Макс. длительность", unit="sec",
        desc_en="Video duration limit (0 = no limit).",
        desc_ru="Лимит длительности видео (0 = без лимита).",
    ),
    RuntimeSettingDef(
        "tiktok.allow_photo_slideshows",
        "tiktok_allow_photo_slideshows",
        "tiktok",
        "allow_photo_slideshows",
        value_type="bool",
        label_en="Allow photo slideshows", label_ru="Фото-слайдшоу",
        desc_en="Allow downloading photo slideshow content.",
        desc_ru="Разрешить скачивание фото-слайдшоу.",
    ),
    RuntimeSettingDef(
        "tiktok.fallback_to_document",
        "tiktok_fallback_to_document",
        "tiktok",
        "fallback_to_document",
        value_type="bool",
        label_en="Fallback to document", label_ru="Документ как fallback",
        desc_en="Send as document if video exceeds limits.",
        desc_ru="Отправлять документом, если видео не проходит лимит.",
    ),
    RuntimeSettingDef(
        "tiktok.carousel_max_items",
        "tiktok_carousel_max_items",
        "tiktok",
        "carousel_max_items",
        value_type="int", min_value=1, max_value=50,
        label_en="Max carousel images", label_ru="Макс. фото в карусели",
        desc_en="Maximum number of images to send from a slideshow (0 = no limit).",
        desc_ru="Максимум фото из слайд-шоу (0 = без лимита).",
    ),
    RuntimeSettingDef(
        "tiktok.carousel_audio_enabled",
        "tiktok_carousel_audio_enabled",
        "tiktok",
        "carousel_audio_enabled",
        value_type="bool",
        label_en="Carousel audio", label_ru="Аудио карусели",
        desc_en="Send audio track along with carousel images when available.",
        desc_ru="Отправлять аудио-дорожку вместе с фото карусели, если доступно.",
    ),

    # -- instagram ----------------------------------------------------------
    RuntimeSettingDef(
        "instagram.max_file_mb",
        "send_video_limit_mb",
        "instagram",
        "max_file_mb",
        value_type="int", min_value=1, max_value=500,
        label_en="Max file", label_ru="Лимит файла", unit="MB",
    ),
    RuntimeSettingDef(
        "instagram.timeout_sec",
        "download_timeout_seconds",
        "instagram",
        "timeout_sec",
        value_type="int", min_value=10, max_value=300,
        label_en="Timeout", label_ru="Таймаут", unit="sec",
    ),
    RuntimeSettingDef(
        "instagram.max_items_per_post",
        "instagram_max_items_per_post",
        "instagram",
        "max_items_per_post",
        value_type="int", min_value=1, max_value=20,
        label_en="Max items per post", label_ru="Элементов на пост",
        desc_en="Max media items from a carousel/post.",
        desc_ru="Макс. элементов из карусели/поста.",
    ),
    RuntimeSettingDef(
        "instagram.allow_reels",
        "instagram_allow_reels",
        "instagram",
        "allow_reels",
        value_type="bool",
        label_en="Allow reels", label_ru="Reels",
    ),
    RuntimeSettingDef(
        "instagram.allow_posts",
        "instagram_allow_posts",
        "instagram",
        "allow_posts",
        value_type="bool",
        label_en="Allow posts", label_ru="Посты",
    ),
    RuntimeSettingDef(
        "instagram.allow_stories",
        "instagram_allow_stories",
        "instagram",
        "allow_stories",
        value_type="bool",
        label_en="Allow stories", label_ru="Stories",
    ),
    RuntimeSettingDef(
        "instagram.fallback_to_document",
        "instagram_fallback_to_document",
        "instagram",
        "fallback_to_document",
        value_type="bool",
        label_en="Fallback to document", label_ru="Документ как fallback",
    ),

    # -- x / twitter --------------------------------------------------------
    RuntimeSettingDef(
        "x.max_file_mb",
        "send_video_limit_mb",
        "x",
        "max_file_mb",
        value_type="int", min_value=1, max_value=500,
        label_en="Max file", label_ru="Лимит файла", unit="MB",
    ),
    RuntimeSettingDef(
        "x.timeout_sec",
        "download_timeout_seconds",
        "x",
        "timeout_sec",
        value_type="int", min_value=10, max_value=300,
        label_en="Timeout", label_ru="Таймаут", unit="sec",
    ),
    RuntimeSettingDef(
        "x.max_items_per_post",
        "x_max_items_per_post",
        "x",
        "max_items_per_post",
        value_type="int", min_value=1, max_value=10,
        label_en="Max items per post", label_ru="Элементов на пост",
        desc_en="Max media attachments from a single post.",
        desc_ru="Макс. медиа-вложений из одного поста.",
    ),
    RuntimeSettingDef(
        "x.allow_gif",
        "x_allow_gif",
        "x",
        "allow_gif",
        value_type="bool",
        label_en="Allow GIF", label_ru="GIF",
    ),
    RuntimeSettingDef(
        "x.allow_video",
        "x_allow_video",
        "x",
        "allow_video",
        value_type="bool",
        label_en="Allow video", label_ru="Видео",
    ),
    RuntimeSettingDef(
        "x.fallback_to_document",
        "x_fallback_to_document",
        "x",
        "fallback_to_document",
        value_type="bool",
        label_en="Fallback to document", label_ru="Документ как fallback",
    ),

    # -- spotify ------------------------------------------------------------
    RuntimeSettingDef(
        "spotify.enabled",
        "spotify_enabled",
        "spotify",
        "enabled",
        value_type="bool",
        label_en="Enabled", label_ru="Включено",
        confirm_on_change=True,
    ),
    RuntimeSettingDef(
        "spotify.download_enabled",
        "spotify_download_enabled",
        "spotify",
        "download_enabled",
        value_type="bool",
        label_en="Download enabled", label_ru="Скачивание",
    ),
    RuntimeSettingDef(
        "spotify.max_file_mb",
        "send_document_limit_mb",
        "spotify",
        "max_file_mb",
        value_type="int", min_value=1, max_value=1999,
        label_en="Max file", label_ru="Лимит файла", unit="MB",
    ),
    RuntimeSettingDef(
        "spotify.track_timeout_sec",
        "spotify_track_timeout_seconds",
        "spotify",
        "track_timeout_sec",
        value_type="int", min_value=10, max_value=300,
        label_en="Track timeout", label_ru="Таймаут трека", unit="sec",
    ),
    RuntimeSettingDef(
        "spotify.api_timeout_sec",
        "spotify_api_timeout_seconds",
        "spotify",
        "api_timeout_sec",
        value_type="int", min_value=5, max_value=60,
        label_en="API timeout", label_ru="Таймаут API", unit="sec",
    ),
    RuntimeSettingDef(
        "spotify.max_tracks_per_album",
        "spotify_lock_max_tracks",
        "spotify",
        "max_tracks_per_album",
        value_type="int", min_value=1, max_value=100,
        label_en="Max tracks per album", label_ru="Треков на альбом",
    ),
    RuntimeSettingDef(
        "spotify.download_concurrency",
        "spotify_download_concurrency",
        "spotify",
        "download_concurrency",
        value_type="int", min_value=1, max_value=5,
        label_en="Download concurrency", label_ru="Одновременных загрузок",
    ),
    RuntimeSettingDef(
        "spotify.metadata_cache_ttl_sec",
        "spotify_meta_cache_ttl_seconds",
        "spotify",
        "metadata_cache_ttl_sec",
        value_type="int", min_value=60, max_value=86400,
        label_en="Metadata cache TTL", label_ru="TTL кеша метаданных", unit="sec",
    ),
    RuntimeSettingDef(
        "spotify.youtube_search_cache_ttl_sec",
        "youtube_search_cache_ttl_seconds",
        "spotify",
        "youtube_search_cache_ttl_sec",
        value_type="int", min_value=60, max_value=604800,
        label_en="YouTube search cache TTL", label_ru="TTL кеша поиска YouTube", unit="sec",
    ),

    # -- soundcloud ---------------------------------------------------------
    RuntimeSettingDef(
        "soundcloud.enabled",
        "soundcloud_enabled",
        "soundcloud",
        "enabled",
        value_type="bool",
        label_en="Enabled", label_ru="Включено",
        confirm_on_change=True,
    ),
    RuntimeSettingDef(
        "soundcloud.download_enabled",
        "soundcloud_download_enabled",
        "soundcloud",
        "download_enabled",
        value_type="bool",
        label_en="Download enabled", label_ru="Скачивание",
    ),
    RuntimeSettingDef(
        "soundcloud.max_file_mb",
        "soundcloud_max_file_mb",
        "soundcloud",
        "max_file_mb",
        value_type="int", min_value=1, max_value=1999,
        label_en="Max file", label_ru="Лимит файла", unit="MB",
    ),
    RuntimeSettingDef(
        "soundcloud.track_timeout_sec",
        "soundcloud_track_timeout_seconds",
        "soundcloud",
        "track_timeout_sec",
        value_type="int", min_value=10, max_value=300,
        label_en="Track timeout", label_ru="Таймаут трека", unit="sec",
    ),
    RuntimeSettingDef(
        "soundcloud.max_tracks_per_playlist",
        "soundcloud_max_tracks",
        "soundcloud",
        "max_tracks_per_playlist",
        value_type="int", min_value=1, max_value=100,
        label_en="Max tracks per playlist", label_ru="Треков в плейлисте",
    ),
    RuntimeSettingDef(
        "soundcloud.metadata_cache_ttl_sec",
        "soundcloud_meta_cache_ttl_seconds",
        "soundcloud",
        "metadata_cache_ttl_sec",
        value_type="int", min_value=60, max_value=86400,
        label_en="Metadata cache TTL", label_ru="TTL кеша метаданных", unit="sec",
    ),
    RuntimeSettingDef(
        "soundcloud.audio_format",
        "soundcloud_dl_output_format",
        "soundcloud",
        "audio_format",
        value_type="enum", allowed_values=("mp3", "opus", "aac", "flac", "wav"),
        label_en="Audio format", label_ru="Формат аудио",
    ),

    # -- pinterest ----------------------------------------------------------
    RuntimeSettingDef(
        "pinterest.enabled",
        "pinterest_enabled",
        "pinterest",
        "enabled",
        value_type="bool",
        label_en="Enabled", label_ru="Включено",
        confirm_on_change=True,
    ),
    RuntimeSettingDef(
        "pinterest.max_file_mb",
        "send_video_limit_mb",
        "pinterest",
        "max_file_mb",
        value_type="int", min_value=1, max_value=500,
        label_en="Max file", label_ru="Лимит файла", unit="MB",
    ),
    RuntimeSettingDef(
        "pinterest.timeout_sec",
        "pinterest_timeout_seconds",
        "pinterest",
        "timeout_sec",
        value_type="int", min_value=10, max_value=300,
        label_en="Timeout", label_ru="Таймаут", unit="sec",
    ),
    RuntimeSettingDef(
        "pinterest.max_items",
        "pinterest_max_items",
        "pinterest",
        "max_items",
        value_type="int", min_value=1, max_value=50,
        label_en="Max items", label_ru="Элементов",
        desc_en="Max pins to process from a board or search.",
        desc_ru="Макс. пинов из доски или поиска.",
    ),
    RuntimeSettingDef(
        "pinterest.download_images",
        "pinterest_download_images",
        "pinterest",
        "download_images",
        value_type="bool",
        label_en="Download images", label_ru="Скачивать изображения",
    ),
    RuntimeSettingDef(
        "pinterest.download_videos",
        "pinterest_download_videos",
        "pinterest",
        "download_videos",
        value_type="bool",
        label_en="Download videos", label_ru="Скачивать видео",
    ),
    RuntimeSettingDef(
        "pinterest.save_metadata",
        "pinterest_save_metadata",
        "pinterest",
        "save_metadata",
        value_type="bool",
        label_en="Save metadata", label_ru="Сохранять метаданные",
    ),
    RuntimeSettingDef(
        "pinterest.use_browser_mode",
        "pinterest_use_browser",
        "pinterest",
        "use_browser_mode",
        value_type="bool",
        label_en="Browser mode", label_ru="Режим браузера",
        desc_en="Use Playwright browser for Pinterest scraping.",
        desc_ru="Использовать Playwright для парсинга Pinterest.",
    ),
)

# ---------------------------------------------------------------------------
# Derived indexes
# ---------------------------------------------------------------------------

_SETTINGS_BY_REDIS_KEY: dict[str, RuntimeSettingDef] = {s.redis_key: s for s in RUNTIME_SETTINGS}
_SETTINGS_BY_SERVICE: dict[str, list[RuntimeSettingDef]] = {}
for _s in RUNTIME_SETTINGS:
    _SETTINGS_BY_SERVICE.setdefault(_s.service, []).append(_s)

_UNSET = object()


def service_settings(service: str) -> list[RuntimeSettingDef]:
    return list(_SETTINGS_BY_SERVICE.get(service, []))


def setting_definition(redis_key: str) -> RuntimeSettingDef | None:
    return _SETTINGS_BY_REDIS_KEY.get(redis_key)


# ---------------------------------------------------------------------------
# Default value helpers
# ---------------------------------------------------------------------------


def _default_value(defn: RuntimeSettingDef, settings_obj: Settings | None = None) -> Any:
    source = settings_obj or settings
    return getattr(source, defn.settings_attr, None)


# ---------------------------------------------------------------------------
# Serialisation helpers
# ---------------------------------------------------------------------------


def _serialise(value: Any) -> str:
    if isinstance(value, bool):
        return "1" if value else "0"
    if isinstance(value, (list, tuple)):
        return ",".join(str(v) for v in value)
    return str(value)


def _deserialise(raw: str | None, defn: RuntimeSettingDef | None) -> Any:
    """Turn a raw Redis hash-field string back into the right Python type."""
    if raw is None:
        return None
    try:
        if defn is None:
            return int(raw)  # backward compat for unknown keys
        if defn.value_type == "bool":
            return raw in ("1", "true", "True", "yes")
        if defn.value_type in ("enum", "list"):
            return tuple(v.strip() for v in raw.split(",") if v.strip())
        return int(raw)
    except (TypeError, ValueError):
        key = defn.redis_key if defn else "<unknown>"
        logger.warning("invalid runtime setting override", key=key, raw=raw)
        return None


# ---------------------------------------------------------------------------
# Validation
# ---------------------------------------------------------------------------


def validate_value(defn: RuntimeSettingDef, raw_value: str) -> str | None:
    """Return an error message (in English) or *None* on success."""
    try:
        if defn.value_type == "bool":
            v = raw_value.strip().lower()
            if v not in ("1", "0", "true", "false", "yes", "no", "on", "off"):
                return "Send 1/0, true/false, yes/no, or on/off."
            return None

        if defn.value_type == "enum":
            v = raw_value.strip()
            if v not in defn.allowed_values:
                allowed = ", ".join(defn.allowed_values)
                return f"Choose from: {allowed}"
            return None

        if defn.value_type == "list":
            parts = [p.strip() for p in raw_value.split(",") if p.strip()]
            if defn.allowed_values:
                invalid = [p for p in parts if p not in defn.allowed_values]
                if invalid:
                    allowed = ", ".join(defn.allowed_values)
                    return f"Invalid values: {', '.join(invalid)}. Allowed: {allowed}"
            return None

        # int
        value = int(raw_value)
        if defn.min_value is not None and value < defn.min_value:
            return f"Minimum value is {defn.min_value}."
        if defn.max_value is not None and value > defn.max_value:
            return f"Maximum value is {defn.max_value}."
        return None

    except ValueError:
        return "Send a valid number."


# ---------------------------------------------------------------------------
# Unicode (display) helpers
# ---------------------------------------------------------------------------


def format_value(value: Any, defn: RuntimeSettingDef, lang: str = "en") -> str:
    """Format a value for display in the admin UI."""
    if value is None:
        return "—"
    if defn.value_type == "bool":
        if lang == "ru":
            return "✅ Вкл" if value else "❌ Выкл"
        return "✅ On" if value else "❌ Off"
    if defn.value_type in ("enum", "list"):
        if isinstance(value, (list, tuple)):
            return ", ".join(str(v) for v in value)
        return str(value)
    # int
    unit = defn.unit
    return f"{value} {unit}".strip() if unit else str(value)


# ---------------------------------------------------------------------------
# CRUD — public API
# ---------------------------------------------------------------------------


def get_runtime_int(redis_key: str, default: int | None = None) -> int:
    """Synchronous read — used inside Celery workers."""
    defn = _SETTINGS_BY_REDIS_KEY.get(redis_key)
    fallback = default if default is not None else (_default_value(defn) if defn else 0)
    try:
        redis_client = get_sync_redis()
        raw = redis_client.hget(REDIS_KEY, redis_key)
        override: int | None = _deserialise(raw, defn) if defn else _deserialise(raw, None)  # type: ignore[arg-type]
        return override if override is not None else fallback
    except Exception:
        record_rpc_failure("redis")
        logger.warning("runtime settings read failed", key=redis_key, exc_info=True)
        return fallback


async def get_runtime_int_async(redis_key: str, default: int | None = None) -> int:
    """Async read — used inside bot handlers."""
    defn = _SETTINGS_BY_REDIS_KEY.get(redis_key)
    fallback = default if default is not None else (_default_value(defn) if defn else 0)
    try:
        redis_client = await get_async_redis()
        raw = await redis_client.hget(REDIS_KEY, redis_key)
        override: int | None = _deserialise(raw, defn) if defn else _deserialise(raw, None)  # type: ignore[arg-type]
        return override if override is not None else fallback
    except Exception:
        record_rpc_failure("redis")
        logger.warning("runtime settings read failed", key=redis_key, exc_info=True)
        return fallback


async def get_runtime_value(redis_key: str) -> Any:
    """Read a runtime value with proper type deserialisation."""
    defn = _SETTINGS_BY_REDIS_KEY.get(redis_key)
    fallback = _default_value(defn) if defn else None
    try:
        redis_client = await get_async_redis()
        raw = await redis_client.hget(REDIS_KEY, redis_key)
        override = _deserialise(raw, defn) if defn else _deserialise(raw, None)
        if override is not None:
            return override
        return fallback
    except Exception:
        record_rpc_failure("redis")
        logger.warning("runtime settings read failed", key=redis_key, exc_info=True)
        return fallback


def get_runtime_value_sync(redis_key: str, default: Any = _UNSET) -> Any:
    """Synchronous version of get_runtime_value, for Celery workers."""
    defn = _SETTINGS_BY_REDIS_KEY.get(redis_key)
    fallback = default if default is not _UNSET else (_default_value(defn) if defn else None)
    try:
        redis_client = get_sync_redis()
        raw = redis_client.hget(REDIS_KEY, redis_key)
        override = _deserialise(raw, defn) if defn else _deserialise(raw, None)
        return override if override is not None else fallback
    except Exception:
        record_rpc_failure("redis")
        logger.warning("runtime settings read failed", key=redis_key, exc_info=True)
        return fallback


def get_runtime_bool(redis_key: str, default: bool | object = _UNSET) -> bool:
    if default is _UNSET:
        return bool(get_runtime_value_sync(redis_key))
    return bool(get_runtime_value_sync(redis_key, default))


def get_runtime_string(redis_key: str) -> str:
    value = get_runtime_value_sync(redis_key)
    if isinstance(value, (list, tuple)):
        return str(value[0]) if value else ""
    return str(value or "")


async def set_runtime_value(redis_key: str, value: Any) -> None:
    """Set a runtime value with proper serialisation."""
    if redis_key not in _SETTINGS_BY_REDIS_KEY:
        raise KeyError(redis_key)
    redis_client = await get_async_redis()
    await redis_client.hset(REDIS_KEY, redis_key, _serialise(value))


async def set_runtime_int(redis_key: str, value: int) -> None:
    """Backward-compatible alias — set an int value."""
    await set_runtime_value(redis_key, value)


async def reset_runtime(redis_key: str | None = None) -> None:
    redis_client = await get_async_redis()
    if redis_key is None:
        await redis_client.delete(REDIS_KEY)
        return
    await redis_client.hdel(REDIS_KEY, redis_key)


async def current_value(defn: RuntimeSettingDef) -> Any:
    return await get_runtime_value(defn.redis_key)


def current_value_sync(defn: RuntimeSettingDef) -> Any:
    return get_runtime_value_sync(defn.redis_key)


async def all_current_values() -> dict[str, Any]:
    values: dict[str, Any] = {}
    for defn in RUNTIME_SETTINGS:
        values[defn.redis_key] = await current_value(defn)
    return values


# ---------------------------------------------------------------------------
# Convenience accessors (used by consumers across the codebase)
# ---------------------------------------------------------------------------


def platform_max_file_mb(platform: str) -> int:
    return get_runtime_int(f"{platform}.max_file_mb")


def platform_download_timeout_seconds(platform: str) -> int:
    return get_runtime_int(f"{platform}.timeout_sec")


def send_document_limit_mb() -> int:
    return get_runtime_int("global.document_limit_mb")


def telegram_bot_upload_limit_mb() -> int:
    return get_runtime_int("global.telegram_upload_limit_mb")


def spotify_track_timeout_seconds() -> int:
    return get_runtime_int("spotify.track_timeout_sec")


def soundcloud_track_timeout_seconds() -> int:
    return get_runtime_int("soundcloud.track_timeout_sec")


def soundcloud_enabled() -> bool:
    return get_runtime_bool("soundcloud.enabled")


def soundcloud_download_enabled(default: bool | object = _UNSET) -> bool:
    return get_runtime_bool("soundcloud.download_enabled", default)


def soundcloud_max_tracks() -> int:
    return get_runtime_int("soundcloud.max_tracks_per_playlist")


def soundcloud_max_file_mb() -> int:
    return get_runtime_int("soundcloud.max_file_mb")


def soundcloud_meta_cache_ttl_seconds() -> int:
    return get_runtime_int("soundcloud.metadata_cache_ttl_sec")


def soundcloud_audio_format() -> str:
    return get_runtime_string("soundcloud.audio_format")


def pinterest_timeout_seconds() -> int:
    return get_runtime_int("pinterest.timeout_sec")


def pinterest_max_file_mb() -> int:
    return get_runtime_int("pinterest.max_file_mb")

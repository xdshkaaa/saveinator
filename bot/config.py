from pydantic_settings import BaseSettings, SettingsConfigDict


class Settings(BaseSettings):
    model_config = SettingsConfigDict(env_file=".env", env_file_encoding="utf-8")

    bot_token: str
    database_url: str = "sqlite+aiosqlite:///./dev.db"
    redis_url: str = "redis://localhost:6379/0"

    webhook_host: str = "https://saveinator-hooks.xdshka.party"
    webhook_path: str = "/webhook"
    webhook_port: int = 8000
    webhook_listen: str = "0.0.0.0"
    webhook_secret_token: str = ""

    celery_broker_url: str = "redis://localhost:6379/0"
    celery_result_backend: str = "redis://localhost:6379/0"

    use_polling: bool = True

    rate_limit_user_per_minute: int = 5
    rate_limit_chat_per_minute: int = 20
    spam_dedup_window_seconds: int = 300
    max_file_size_mb: int = 500
    send_video_limit_mb: int = 50
    send_document_limit_mb: int = 1999
    telegram_bot_upload_limit_mb: int = 50
    download_timeout_seconds: int = 60
    youtube_max_file_size_mb: int = 1999
    youtube_download_timeout_seconds: int = 600
    youtube_allowed_qualities: str = "1080,720,480"
    youtube_default_quality: str = "ask"
    youtube_allowed_ratios: str = "16_9,21_9,9_16"
    youtube_transcode_enabled: bool = True
    youtube_max_duration_sec: int = 0

    tiktok_max_duration_sec: int = 0
    tiktok_allow_photo_slideshows: bool = True
    tiktok_fallback_to_document: bool = True
    tiktok_carousel_max_items: int = 20
    tiktok_carousel_audio_enabled: bool = True
    tiktok_cookies_path: str = ""
    tiktok_cookies_from_browser: str = "chrome"
    tiktok_cookies_refresh_enabled: bool = True
    tiktok_cookies_refresh_url: str = "https://vt.tiktok.com/ZSCFGyN3g/"

    instagram_max_items_per_post: int = 10
    instagram_allow_reels: bool = True
    instagram_allow_posts: bool = True
    instagram_allow_stories: bool = True
    instagram_fallback_to_document: bool = True

    x_max_items_per_post: int = 4
    x_allow_gif: bool = True
    x_allow_video: bool = True
    x_fallback_to_document: bool = True

    broadcast_delay_ms: int = 50
    broadcast_batch_size: int = 20

    sentry_dsn: str = ""
    log_level: str = "INFO"

    spotify_enabled: bool = False
    spotify_client_id: str = ""
    spotify_client_secret: str = ""
    spotify_api_timeout_seconds: int = 15
    spotify_download_enabled: bool = True
    spotify_track_timeout_seconds: int = 60
    spotify_dl_output_format: str = "mp3"
    spotify_lock_max_tracks: int = 50
    spotify_download_concurrency: int = 1
    spotify_meta_cache_ttl_seconds: int = 3600
    youtube_search_cache_ttl_seconds: int = 604800

    soundcloud_enabled: bool = True
    soundcloud_download_enabled: bool = False
    soundcloud_download_concurrency: int = 1
    soundcloud_track_timeout_seconds: int = 30
    soundcloud_max_tracks: int = 100
    soundcloud_dl_output_format: str = "mp3"
    soundcloud_max_file_mb: int = 50
    soundcloud_meta_cache_ttl_seconds: int = 3600

    pinterest_enabled: bool = True
    pinterest_timeout_seconds: int = 30
    pinterest_max_items: int = 1
    pinterest_download_images: bool = True
    pinterest_download_videos: bool = True
    pinterest_api_timeout_seconds: int = 10
    pinterest_use_browser: bool = False
    pinterest_cookies_path: str = ""
    pinterest_save_metadata: bool = True
    download_api_enabled: bool = True

    metrics_enabled: bool = True
    metrics_host: str = "0.0.0.0"
    metrics_port: int = 9101
    worker_metrics_port: int = 9102

    admin_telegram_id: int = 0


settings = Settings()

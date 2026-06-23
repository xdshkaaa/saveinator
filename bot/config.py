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
    spotify_download_concurrency: int = 2
    spotify_meta_cache_ttl_seconds: int = 3600
    youtube_search_cache_ttl_seconds: int = 604800

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

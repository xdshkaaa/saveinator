from pydantic_settings import BaseSettings, SettingsConfigDict


class Settings(BaseSettings):
    model_config = SettingsConfigDict(env_file=".env", env_file_encoding="utf-8")

    bot_token: str
    database_url: str = "sqlite+aiosqlite:///./dev.db"
    redis_url: str = "redis://localhost:6379/0"

    webhook_host: str = "https://xdshka.party"
    webhook_path: str = "/webhook"
    webhook_port: int = 8000
    webhook_listen: str = "0.0.0.0"

    celery_broker_url: str = "redis://localhost:6379/0"
    celery_result_backend: str = "redis://localhost:6379/0"

    use_polling: bool = True

    rate_limit_user_per_minute: int = 5
    rate_limit_chat_per_minute: int = 20
    spam_dedup_window_seconds: int = 300
    max_file_size_mb: int = 500
    send_video_limit_mb: int = 50
    send_document_limit_mb: int = 100

    sentry_dsn: str = ""
    log_level: str = "INFO"


settings = Settings()

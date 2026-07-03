from sqlalchemy import BigInteger, String, DateTime, Enum, ForeignKey, Text, Index, Integer, UniqueConstraint, JSON
from sqlalchemy.dialects.postgresql import JSONB
from sqlalchemy.orm import DeclarativeBase, Mapped, mapped_column, relationship
from datetime import datetime, UTC
import enum


def utc_now_naive() -> datetime:
    return datetime.now(UTC).replace(tzinfo=None)


class Base(DeclarativeBase):
    pass


class Platform(str, enum.Enum):
    YOUTUBE = "youtube"
    TIKTOK = "tiktok"
    INSTAGRAM = "instagram"
    X = "x"
    SPOTIFY = "spotify"
    SOUNDCLOUD = "soundcloud"
    PINTEREST = "pinterest"
    UNKNOWN = "unknown"


class DownloadStatus(str, enum.Enum):
    QUEUED = "queued"
    FETCHING = "fetching_formats"
    DOWNLOADING = "downloading"
    TRANSCODING = "transcoding"
    SENDING = "sending"
    COMPLETED = "completed"
    FAILED = "failed"


class Language(str, enum.Enum):
    EN = "en"
    RU = "ru"
    KK = "kk"


class Chat(Base):
    __tablename__ = "chats"

    id: Mapped[int] = mapped_column(BigInteger, primary_key=True)
    title: Mapped[str | None] = mapped_column(String(255))
    type: Mapped[str] = mapped_column(String(16))
    created_at: Mapped[datetime] = mapped_column(DateTime, default=utc_now_naive)

    downloads: Mapped[list["Download"]] = relationship(back_populates="chat")


class User(Base):
    __tablename__ = "users"

    id: Mapped[int] = mapped_column(BigInteger, primary_key=True)
    username: Mapped[str | None] = mapped_column(String(64))
    first_name: Mapped[str | None] = mapped_column(String(128))
    language: Mapped[Language] = mapped_column(Enum(Language), default=Language.EN)
    created_at: Mapped[datetime] = mapped_column(DateTime, default=utc_now_naive)

    downloads: Mapped[list["Download"]] = relationship(back_populates="user")


class Download(Base):
    __tablename__ = "downloads"

    id: Mapped[int] = mapped_column(Integer, primary_key=True, autoincrement=True)
    user_id: Mapped[int] = mapped_column(BigInteger, ForeignKey("users.id"))
    chat_id: Mapped[int] = mapped_column(BigInteger, ForeignKey("chats.id"))
    url: Mapped[str] = mapped_column(Text)
    platform: Mapped[Platform] = mapped_column(Enum(Platform))
    format_id: Mapped[str | None] = mapped_column(String(64))
    quality_label: Mapped[str | None] = mapped_column(String(32))
    file_size: Mapped[int | None] = mapped_column(BigInteger)
    status: Mapped[DownloadStatus] = mapped_column(Enum(DownloadStatus), default=DownloadStatus.QUEUED)
    error_message: Mapped[str | None] = mapped_column(Text)
    created_at: Mapped[datetime] = mapped_column(DateTime, default=utc_now_naive)
    completed_at: Mapped[datetime | None] = mapped_column(DateTime)

    user: Mapped["User"] = relationship(back_populates="downloads")
    chat: Mapped["Chat"] = relationship(back_populates="downloads")

    __table_args__ = (
        Index("ix_downloads_user_created", "user_id", "created_at"),
        Index("ix_downloads_chat_created", "chat_id", "created_at"),
        Index("ix_downloads_status", "status"),
    )


class MusicReleaseMetadata(Base):
    __tablename__ = "music_release_metadata"

    id: Mapped[int] = mapped_column(Integer, primary_key=True, autoincrement=True)
    platform: Mapped[Platform] = mapped_column(Enum(Platform))
    source_id: Mapped[str] = mapped_column(String(128))
    release_type: Mapped[str] = mapped_column(String(32))
    canonical_url: Mapped[str] = mapped_column(Text)
    title: Mapped[str] = mapped_column(String(512))
    artist: Mapped[str] = mapped_column(String(512))
    track_count: Mapped[int] = mapped_column(Integer, default=0)
    payload: Mapped[dict] = mapped_column(JSON().with_variant(JSONB, "postgresql"))
    first_fetched_at: Mapped[datetime] = mapped_column(DateTime, default=utc_now_naive)
    last_fetched_at: Mapped[datetime] = mapped_column(DateTime, default=utc_now_naive)

    __table_args__ = (
        UniqueConstraint("platform", "source_id", name="uq_music_release_platform_source"),
        Index("ix_music_release_platform_source", "platform", "source_id"),
    )


class BannedLink(Base):
    __tablename__ = "banned_links"

    id: Mapped[int] = mapped_column(Integer, primary_key=True, autoincrement=True)
    url_hash: Mapped[str] = mapped_column(String(64), unique=True, index=True)
    reason: Mapped[str | None] = mapped_column(String(255))
    created_at: Mapped[datetime] = mapped_column(DateTime, default=utc_now_naive)


class UserSettings(Base):
    __tablename__ = "user_settings"

    user_id: Mapped[int] = mapped_column(
        BigInteger, ForeignKey("users.id"), primary_key=True
    )
    youtube_quality: Mapped[str] = mapped_column(
        String(16), default="ask", server_default="ask"
    )
    youtube_ratio: Mapped[str] = mapped_column(
        String(16), default="ask", server_default="ask"
    )


class BroadcastStatus(str, enum.Enum):
    DRAFT = "draft"
    QUEUED = "queued"
    RUNNING = "running"
    COMPLETED = "completed"
    FAILED = "failed"
    CANCELLED = "cancelled"


class BroadcastAudience(str, enum.Enum):
    ALL = "all"
    RU = "ru"
    EN = "en"
    ACTIVE = "active"


class BroadcastDeliveryStatus(str, enum.Enum):
    PENDING = "pending"
    SENT = "sent"
    FAILED = "failed"
    BLOCKED = "blocked"


class Broadcast(Base):
    __tablename__ = "broadcasts"

    id: Mapped[int] = mapped_column(Integer, primary_key=True, autoincrement=True)
    admin_id: Mapped[int] = mapped_column(BigInteger)
    text: Mapped[str] = mapped_column(Text)
    audience: Mapped[BroadcastAudience] = mapped_column(
        Enum(BroadcastAudience), default=BroadcastAudience.ALL
    )
    status: Mapped[BroadcastStatus] = mapped_column(
        Enum(BroadcastStatus), default=BroadcastStatus.DRAFT
    )
    total_recipients: Mapped[int] = mapped_column(Integer, default=0)
    sent_count: Mapped[int] = mapped_column(Integer, default=0)
    failed_count: Mapped[int] = mapped_column(Integer, default=0)
    blocked_count: Mapped[int] = mapped_column(Integer, default=0)
    created_at: Mapped[datetime] = mapped_column(DateTime, default=utc_now_naive)
    started_at: Mapped[datetime | None] = mapped_column(DateTime, nullable=True)
    finished_at: Mapped[datetime | None] = mapped_column(DateTime, nullable=True)

    deliveries: Mapped[list["BroadcastDelivery"]] = relationship(back_populates="broadcast")


class BroadcastDelivery(Base):
    __tablename__ = "broadcast_deliveries"

    id: Mapped[int] = mapped_column(Integer, primary_key=True, autoincrement=True)
    broadcast_id: Mapped[int] = mapped_column(
        Integer, ForeignKey("broadcasts.id"), nullable=False
    )
    user_id: Mapped[int] = mapped_column(BigInteger)
    status: Mapped[BroadcastDeliveryStatus] = mapped_column(
        Enum(BroadcastDeliveryStatus), default=BroadcastDeliveryStatus.PENDING
    )
    error_message: Mapped[str | None] = mapped_column(Text, nullable=True)
    sent_at: Mapped[datetime | None] = mapped_column(DateTime, nullable=True)

    broadcast: Mapped["Broadcast"] = relationship(back_populates="deliveries")

    __table_args__ = (
        Index("ix_broadcast_deliveries_broadcast_id", "broadcast_id"),
        Index("ix_broadcast_deliveries_user_id", "user_id"),
    )

from sqlalchemy import BigInteger, String, DateTime, Enum, ForeignKey, Text, Index, Integer
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


class BannedLink(Base):
    __tablename__ = "banned_links"

    id: Mapped[int] = mapped_column(Integer, primary_key=True, autoincrement=True)
    url_hash: Mapped[str] = mapped_column(String(64), unique=True, index=True)
    reason: Mapped[str | None] = mapped_column(String(255))
    created_at: Mapped[datetime] = mapped_column(DateTime, default=utc_now_naive)

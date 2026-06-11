from dataclasses import dataclass, field
from datetime import UTC, datetime
from enum import Enum
from typing import Literal


class PinterestUrlType(str, Enum):
    PIN = "pin"
    BOARD = "board"
    SHORT = "short"
    UNKNOWN = "unknown"


MediaType = Literal["image", "video"]


@dataclass(frozen=True)
class ParsedPinterestUrl:
    url: str
    url_type: PinterestUrlType


@dataclass
class PinterestMediaItem:
    source_url: str
    media_type: MediaType
    title: str | None
    description: str | None
    original_media_url: str
    file_path: str
    file_size: int
    created_at: datetime = field(
        default_factory=lambda: datetime.now(UTC).replace(tzinfo=None)
    )

    def to_dict(self) -> dict:
        return {
            "source_url": self.source_url,
            "media_type": self.media_type,
            "title": self.title,
            "description": self.description,
            "original_media_url": self.original_media_url,
            "file_path": self.file_path,
            "file_size": self.file_size,
            "created_at": self.created_at.isoformat() + "Z",
        }


@dataclass
class PinterestDownloadResult:
    url: str
    url_type: PinterestUrlType
    items: list[PinterestMediaItem] = field(default_factory=list)
    errors: list[str] = field(default_factory=list)

    def to_dict(self) -> dict:
        return {
            "url": self.url,
            "url_type": self.url_type.value,
            "items": [item.to_dict() for item in self.items],
            "errors": self.errors,
            "count": len(self.items),
        }

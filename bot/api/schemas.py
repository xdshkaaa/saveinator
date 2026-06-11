from pydantic import BaseModel, Field, field_validator, model_validator


class PinterestDownloadRequest(BaseModel):
    url: str = Field(min_length=1)
    limit: int = Field(default=10, ge=1, le=50)
    download_videos: bool = Field(default=True, alias="downloadVideos")
    download_images: bool = Field(default=True, alias="downloadImages")

    model_config = {"populate_by_name": True}

    @field_validator("url")
    @classmethod
    def strip_url(cls, value: str) -> str:
        stripped = value.strip()
        if not stripped:
            raise ValueError("url must not be empty")
        return stripped

    @model_validator(mode="after")
    def require_media_type(self) -> "PinterestDownloadRequest":
        if not self.download_images and not self.download_videos:
            raise ValueError(
                "At least one of downloadImages or downloadVideos must be true"
            )
        return self

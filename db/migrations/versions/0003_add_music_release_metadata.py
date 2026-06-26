"""Add music_release_metadata table

Revision ID: 0003
Revises: 0002
Create Date: 2026-06-26
"""
from typing import Sequence, Union
from alembic import op
import sqlalchemy as sa
from sqlalchemy.dialects.postgresql import JSONB


revision: str = "0003"
down_revision: Union[str, None] = "0002"
branch_labels: Union[str, None] = None
depends_on: Union[str, None] = None

platform_enum = sa.Enum(
    "youtube",
    "tiktok",
    "instagram",
    "x",
    "spotify",
    "soundcloud",
    "pinterest",
    "unknown",
    name="platform",
    create_type=False,
)


def upgrade() -> None:
    op.create_table(
        "music_release_metadata",
        sa.Column("id", sa.Integer(), autoincrement=True, nullable=False),
        sa.Column("platform", platform_enum, nullable=False),
        sa.Column("source_id", sa.String(128), nullable=False),
        sa.Column("release_type", sa.String(32), nullable=False),
        sa.Column("canonical_url", sa.Text(), nullable=False),
        sa.Column("title", sa.String(512), nullable=False),
        sa.Column("artist", sa.String(512), nullable=False),
        sa.Column("track_count", sa.Integer(), nullable=False, server_default="0"),
        sa.Column("payload", JSONB(), nullable=False),
        sa.Column("first_fetched_at", sa.DateTime(), nullable=False),
        sa.Column("last_fetched_at", sa.DateTime(), nullable=False),
        sa.PrimaryKeyConstraint("id"),
        sa.UniqueConstraint("platform", "source_id", name="uq_music_release_platform_source"),
    )
    op.create_index(
        "ix_music_release_platform_source",
        "music_release_metadata",
        ["platform", "source_id"],
    )


def downgrade() -> None:
    op.drop_index("ix_music_release_platform_source", table_name="music_release_metadata")
    op.drop_table("music_release_metadata")

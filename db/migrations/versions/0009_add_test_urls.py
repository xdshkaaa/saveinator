"""Add test_urls table (admin URL test runs from dash)

Revision ID: 0009
Revises: 0008
Create Date: 2026-09-02
"""
from typing import Sequence, Union
from alembic import op


revision: str = "0009"
down_revision: Union[str, None] = "0008"
branch_labels: Union[str, None] = None
depends_on: Union[str, None] = None


def upgrade() -> None:
    op.execute("""
        CREATE TABLE IF NOT EXISTS test_urls (
            id SERIAL PRIMARY KEY,
            url TEXT NOT NULL UNIQUE,
            platform VARCHAR(32) NOT NULL,
            status VARCHAR(16) NOT NULL DEFAULT 'PENDING',
            error_message TEXT,
            media_type VARCHAR(16),
            file_size BIGINT,
            duration_ms INTEGER,
            created_at TIMESTAMP NOT NULL DEFAULT now(),
            checked_at TIMESTAMP,
            updated_at TIMESTAMP NOT NULL DEFAULT now()
        )
    """)
    op.execute(
        "CREATE INDEX IF NOT EXISTS ix_test_urls_status_updated "
        "ON test_urls (status, updated_at)"
    )


def downgrade() -> None:
    op.execute("DROP TABLE IF EXISTS test_urls")

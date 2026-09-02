"""Add TWITCH to platform enum

Revision ID: 0008
Revises: 0007
Create Date: 2026-09-02
"""
from typing import Sequence, Union
from alembic import op
import sqlalchemy as sa


revision: str = "0008"
down_revision: Union[str, None] = "0007"
branch_labels: Union[str, None] = None
depends_on: Union[str, None] = None


def upgrade() -> None:
    op.execute("COMMIT")
    op.execute("ALTER TYPE platform ADD VALUE IF NOT EXISTS 'TWITCH'")


def downgrade() -> None:
    pass

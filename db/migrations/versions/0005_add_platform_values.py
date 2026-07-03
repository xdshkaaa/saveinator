"""Add SPOTIFY, SOUNDCLOUD, PINTEREST to platform enum

Revision ID: 0005
Revises: 0004
Create Date: 2026-07-04
"""
from typing import Sequence, Union
from alembic import op
import sqlalchemy as sa


revision: str = "0005"
down_revision: Union[str, None] = "0004"
branch_labels: Union[str, None] = None
depends_on: Union[str, None] = None


def upgrade() -> None:
    op.execute("COMMIT")
    op.execute("ALTER TYPE platform ADD VALUE IF NOT EXISTS 'SPOTIFY'")
    op.execute("ALTER TYPE platform ADD VALUE IF NOT EXISTS 'SOUNDCLOUD'")
    op.execute("ALTER TYPE platform ADD VALUE IF NOT EXISTS 'PINTEREST'")


def downgrade() -> None:
    pass

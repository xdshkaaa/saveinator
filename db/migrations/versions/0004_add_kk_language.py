"""Add KK to language enum

Revision ID: 0004
Revises: 0003
Create Date: 2026-07-03
"""
from typing import Sequence, Union
from alembic import op
import sqlalchemy as sa


revision: str = "0004"
down_revision: Union[str, None] = "0003"
branch_labels: Union[str, None] = None
depends_on: Union[str, None] = None


def upgrade() -> None:
    op.execute("ALTER TYPE language ADD VALUE IF NOT EXISTS 'KK'")


def downgrade() -> None:
    pass

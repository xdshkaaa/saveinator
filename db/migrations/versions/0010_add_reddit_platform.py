"""Add REDDIT to platform enum

Revision ID: 0010
Revises: 0009
Create Date: 2026-09-04
"""
from typing import Sequence, Union
from alembic import op
import sqlalchemy as sa


revision: str = "0010"
down_revision: Union[str, None] = "0009"
branch_labels: Union[str, None] = None
depends_on: Union[str, None] = None


def upgrade() -> None:
    op.execute("COMMIT")
    op.execute("ALTER TYPE platform ADD VALUE IF NOT EXISTS 'REDDIT'")


def downgrade() -> None:
    pass

"""Add broadcasts and broadcast_deliveries tables

Revision ID: 0002
Revises: 0001
Create Date: 2026-06-23
"""
from typing import Sequence, Union
from alembic import op
import sqlalchemy as sa


revision: str = "0002"
down_revision: Union[str, None] = "0001"
branch_labels: Union[str, None] = None
depends_on: Union[str, None] = None


def upgrade() -> None:
    op.create_table(
        "broadcasts",
        sa.Column("id", sa.Integer(), autoincrement=True, nullable=False),
        sa.Column("admin_id", sa.BigInteger(), nullable=False),
        sa.Column("text", sa.Text(), nullable=False),
        sa.Column("audience", sa.Enum("ALL", "RU", "EN", "ACTIVE", name="broadcastaudience"), nullable=False, server_default="ALL"),
        sa.Column("status", sa.Enum("DRAFT", "QUEUED", "RUNNING", "COMPLETED", "FAILED", "CANCELLED", name="broadcaststatus"), nullable=False, server_default="DRAFT"),
        sa.Column("total_recipients", sa.Integer(), nullable=False, server_default="0"),
        sa.Column("sent_count", sa.Integer(), nullable=False, server_default="0"),
        sa.Column("failed_count", sa.Integer(), nullable=False, server_default="0"),
        sa.Column("blocked_count", sa.Integer(), nullable=False, server_default="0"),
        sa.Column("created_at", sa.DateTime(), nullable=False),
        sa.Column("started_at", sa.DateTime(), nullable=True),
        sa.Column("finished_at", sa.DateTime(), nullable=True),
        sa.PrimaryKeyConstraint("id"),
    )

    op.create_table(
        "broadcast_deliveries",
        sa.Column("id", sa.Integer(), autoincrement=True, nullable=False),
        sa.Column("broadcast_id", sa.Integer(), nullable=False),
        sa.Column("user_id", sa.BigInteger(), nullable=False),
        sa.Column("status", sa.Enum("PENDING", "SENT", "FAILED", "BLOCKED", name="broadcastdeliverystatus"), nullable=False, server_default="PENDING"),
        sa.Column("error_message", sa.Text(), nullable=True),
        sa.Column("sent_at", sa.DateTime(), nullable=True),
        sa.ForeignKeyConstraint(["broadcast_id"], ["broadcasts.id"],),
        sa.PrimaryKeyConstraint("id"),
    )

    op.create_index("ix_broadcast_deliveries_broadcast_id", "broadcast_deliveries", ["broadcast_id"])
    op.create_index("ix_broadcast_deliveries_user_id", "broadcast_deliveries", ["user_id"])


def downgrade() -> None:
    op.drop_table("broadcast_deliveries")
    op.drop_table("broadcasts")
    sa.Enum(name="broadcastaudience").drop(op.get_bind(), checkfirst=True)
    sa.Enum(name="broadcaststatus").drop(op.get_bind(), checkfirst=True)
    sa.Enum(name="broadcastdeliverystatus").drop(op.get_bind(), checkfirst=True)

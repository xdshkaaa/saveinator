"""Add bot scope: bots table, bot_id on users/downloads, per-bot language

Revision ID: 0006
Revises: 0005
Create Date: 2026-07-04
"""
from typing import Sequence, Union
from alembic import op
import sqlalchemy as sa


revision: str = "0006"
down_revision: Union[str, None] = "0005"
branch_labels: Union[str, None] = None
depends_on: Union[str, None] = None

KNOWN_BOTS = ("saveinator", "pinterest", "pinterest_kz", "spotify")


def upgrade() -> None:
    op.create_table(
        "bots",
        sa.Column("slug", sa.String(32), primary_key=True),
        sa.Column("created_at", sa.DateTime(), nullable=False, server_default=sa.text("now()")),
    )
    for slug in KNOWN_BOTS:
        op.execute(f"INSERT INTO bots (slug) VALUES ('{slug}') ON CONFLICT DO NOTHING")

    op.add_column("users", sa.Column("bot_id", sa.String(32), sa.ForeignKey("bots.slug"), nullable=True))
    op.add_column("downloads", sa.Column("bot_id", sa.String(32), sa.ForeignKey("bots.slug"), nullable=True))
    op.create_index("ix_downloads_bot_id", "downloads", ["bot_id"])

    # Per-bot user language / membership. users.language stays as fallback.
    op.create_table(
        "user_bot_settings",
        sa.Column("user_id", sa.BigInteger(), sa.ForeignKey("users.id"), primary_key=True),
        sa.Column("bot_id", sa.String(32), sa.ForeignKey("bots.slug"), primary_key=True),
        sa.Column("language", sa.Enum(name="language", create_type=False), nullable=False, server_default="EN"),
        sa.Column("created_at", sa.DateTime(), nullable=False, server_default=sa.text("now()")),
    )

    # Backfill by download platform; pinterest_kz history is indistinguishable
    # from pinterest and lands in 'pinterest'.
    op.execute("UPDATE downloads SET bot_id = 'pinterest' WHERE platform = 'PINTEREST'")
    op.execute("UPDATE downloads SET bot_id = 'spotify' WHERE platform = 'SPOTIFY'")
    op.execute("UPDATE downloads SET bot_id = 'saveinator' WHERE bot_id IS NULL")

    op.execute("""
        INSERT INTO user_bot_settings (user_id, bot_id, language)
        SELECT DISTINCT d.user_id, d.bot_id, u.language
        FROM downloads d JOIN users u ON u.id = d.user_id
        WHERE d.bot_id IS NOT NULL
        ON CONFLICT DO NOTHING
    """)
    op.execute("""
        INSERT INTO user_bot_settings (user_id, bot_id, language)
        SELECT id, 'saveinator', language FROM users
        ON CONFLICT DO NOTHING
    """)


def downgrade() -> None:
    op.drop_table("user_bot_settings")
    op.drop_index("ix_downloads_bot_id", table_name="downloads")
    op.drop_column("downloads", "bot_id")
    op.drop_column("users", "bot_id")
    op.drop_table("bots")

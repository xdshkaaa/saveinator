"""Watermark unlock: no_watermark user setting + purchases ledger (Telegram Stars)

Revision ID: 0007
Revises: 0006
Create Date: 2026-09-02
"""
from typing import Sequence, Union
from alembic import op


revision: str = "0007"
down_revision: Union[str, None] = "0006"
branch_labels: Union[str, None] = None
depends_on: Union[str, None] = None


def upgrade() -> None:
    op.execute(
        "ALTER TABLE user_settings ADD COLUMN IF NOT EXISTS no_watermark BOOLEAN NOT NULL DEFAULT FALSE"
    )

    # One-time Telegram Stars purchases. telegram_payment_charge_id is unique so
    # a redelivered successful_payment cannot double-record a purchase, and it
    # doubles as the refund handle for Bot API refundStarPayment.
    op.execute("""
        CREATE TABLE IF NOT EXISTS purchases (
            id SERIAL PRIMARY KEY,
            user_id BIGINT NOT NULL REFERENCES users(id),
            product VARCHAR(32) NOT NULL,
            stars_amount INT NOT NULL,
            currency VARCHAR(8) NOT NULL DEFAULT 'XTR',
            telegram_payment_charge_id TEXT NOT NULL UNIQUE,
            created_at TIMESTAMP NOT NULL DEFAULT now()
        )
    """)
    op.execute(
        "CREATE INDEX IF NOT EXISTS ix_purchases_user ON purchases (user_id, product)"
    )


def downgrade() -> None:
    op.drop_index("ix_purchases_user", table_name="purchases")
    op.drop_table("purchases")
    op.drop_column("user_settings", "no_watermark")

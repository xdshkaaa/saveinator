"""Admin action logging — writes setting changes to the bot log."""

import structlog
from typing import Any

logger = structlog.get_logger()


def log_setting_change(
    admin_id: int,
    key: str,
    old_value: Any,
    new_value: Any,
) -> None:
    """Log an admin setting change."""
    logger.info(
        "admin setting changed",
        admin_id=admin_id,
        key=key,
        old_value=old_value,
        new_value=new_value,
    )


def log_broadcast_action(
    admin_id: int,
    broadcast_id: int,
    action: str,
    **extra: Any,
) -> None:
    """Log a broadcast-related admin action."""
    logger.info(
        f"broadcast {action}",
        admin_id=admin_id,
        broadcast_id=broadcast_id,
        **extra,
    )

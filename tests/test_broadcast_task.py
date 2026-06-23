import asyncio

import pytest

from bot.services.broadcast_service import create_broadcast, get_broadcast
from db.models import BroadcastStatus
from workers.broadcast_task import execute_broadcast


async def test_execute_broadcast_marks_broadcast_failed_when_send_crashes(monkeypatch):
    broadcast = await create_broadcast(1, "boom")

    async def _crash(*_args, **_kwargs):
        raise RuntimeError("send crashed")

    monkeypatch.setattr("workers.broadcast_task._send_broadcast", _crash)

    with pytest.raises(RuntimeError, match="send crashed"):
        await asyncio.to_thread(execute_broadcast.run, broadcast.id, "all", [100])

    updated = await get_broadcast(broadcast.id)
    assert updated is not None
    assert updated.status == BroadcastStatus.FAILED
    assert updated.finished_at is not None

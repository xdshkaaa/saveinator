import os
import time
from bot.services.tempfiles import tempfile_manager, sweep_stale


class TestTempfiles:
    def test_context_manager_creates_and_cleans(self):
        task_id = "test-task-123"
        with tempfile_manager(task_id) as task_dir:
            assert task_dir.exists()
            assert task_dir.is_dir()
            (task_dir / "test.txt").write_text("data")
        assert not task_dir.exists()

    def test_sweep_stale_leaves_recent(self):
        with tempfile_manager("recent-task") as task_dir:
            (task_dir / "keep.txt").write_text("keep")
            sweep_stale(older_than_seconds=999999)
            assert task_dir.exists()

    def test_sweep_stale_removes_old(self):
        with tempfile_manager("old-task") as task_dir:
            (task_dir / "old.txt").write_text("old")
            old_mtime = time.time() - 999999
            os.utime(task_dir, (old_mtime, old_mtime))
            sweep_stale(older_than_seconds=1)
            assert not task_dir.exists()

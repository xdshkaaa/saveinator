import os
import shutil
import tempfile
import atexit
from pathlib import Path
from contextlib import contextmanager
from typing import Generator

import structlog

logger = structlog.get_logger()

TEMP_ROOT = Path("/tmp/ytbot")
TEMP_ROOT.mkdir(parents=True, exist_ok=True)


def _sweep_temp_root():
    if TEMP_ROOT.exists():
        for path in TEMP_ROOT.iterdir():
            try:
                if path.is_dir():
                    shutil.rmtree(path, ignore_errors=True)
                else:
                    path.unlink(missing_ok=True)
            except Exception:
                logger.warning("Failed to clean temp path", path=str(path))


atexit.register(_sweep_temp_root)


@contextmanager
def tempfile_manager(task_id: str) -> Generator[Path, None, None]:
    task_dir = TEMP_ROOT / task_id
    task_dir.mkdir(parents=True, exist_ok=True)
    try:
        yield task_dir
    finally:
        try:
            shutil.rmtree(task_dir, ignore_errors=True)
        except Exception:
            logger.warning("Failed to clean task dir", task_id=task_id)


def sweep_stale(older_than_seconds: int = 3600):
    import time
    now = time.time()
    if not TEMP_ROOT.exists():
        return
    for path in TEMP_ROOT.iterdir():
        try:
            mtime = path.stat().st_mtime
            if now - mtime > older_than_seconds:
                if path.is_dir():
                    shutil.rmtree(path, ignore_errors=True)
                    logger.info("Swept stale temp dir", path=str(path))
                else:
                    path.unlink(missing_ok=True)
        except FileNotFoundError:
            pass
        except Exception:
            logger.warning("Sweep error", path=str(path))

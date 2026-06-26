# AGENTS.md

## Cursor Cloud specific instructions

Saveinator is a single Telegram-bot product made of two Python processes that share the same
codebase + config: the **bot** (`bot.main`, aiohttp + aiogram) and the **Celery worker**
(`workers.app`). Both depend on **Redis** (Celery broker/result backend + rate limiting) and a
SQL database. Standard run/test/build commands live in `README.md` ("Run locally" / "Tests")
and `pyproject.toml`; this section only adds non-obvious cloud caveats.

### Environment
- Dependency manager is **uv** (already installed and on `PATH` via `~/.bashrc`/`~/.profile`).
  The startup update script runs `uv sync --extra dev`; the venv is `/workspace/.venv`.
- `ffmpeg` (required by yt-dlp), Redis, and PostgreSQL 16 are installed at the system level.
- A local `.env` is used (dummy `BOT_TOKEN`, SQLite DB, `localhost` Redis). The real
  `BOT_TOKEN` from @BotFather is the only hard-required secret for the live Telegram flow.

### Starting services (not done by the update script)
- Redis is not auto-started. Start it once per session:
  `redis-server --daemonize yes --save "" --appendonly no` (verify with `redis-cli ping`).
- The Celery worker needs `PROMETHEUS_MULTIPROC_DIR` pointing at an existing directory, or its
  on-ready metrics server raises `ValueError: env PROMETHEUS_MULTIPROC_DIR is not set` (the
  worker still becomes "ready", but worker metrics on `:9102` won't start). Run it as:
  `mkdir -p /tmp/prometheus_multiproc && PROMETHEUS_MULTIPROC_DIR=/tmp/prometheus_multiproc uv run celery -A workers.app worker --loglevel=info`
  (`docker-compose.yml` sets this same var.)
- The Telegram bot (`bot.main`) requires a valid `BOT_TOKEN`; in polling mode it connects to
  Telegram on startup and a dummy token fails with an Unauthorized error.

### Download HTTP API without a Telegram token
- The Pinterest download API (`POST /download/pinterest`) and `/health` + `/metrics` can be
  served WITHOUT a real bot token by running only the metrics/API server:
  `uv run python -c "import asyncio; from bot.metrics_server import run_metrics_server_background; asyncio.run(run_metrics_server_background())"`
  It listens on `METRICS_PORT` (default `9101`). This is the fastest way to exercise core
  media-download functionality end to end in this environment.
  Example: `curl -X POST http://127.0.0.1:9101/download/pinterest -H 'Content-Type: application/json' -d '{"url":"https://www.pinterest.com/pin/<id>/","limit":1}'`

### Database caveats (pre-existing, do NOT mistake for env breakage)
- `alembic upgrade head` does NOT produce a complete schema on a fresh DB. Migrations only
  create `user_settings`, `broadcasts`, `broadcast_deliveries`, and `music_release_metadata`,
  but `db/models.py` also defines `users`, `chats`, `downloads`, `banned_links` and FKs to
  `users`. On PostgreSQL the upgrade fails (`relation "users" does not exist`); on the default
  SQLite it silently skips the missing FK and then fails at migration `0003` because that
  revision uses Postgres-only `JSONB`/`platform` enum. Treat this as an app-level migration gap.
- For a working dev DB, bypass the incomplete migrations and build the full schema directly
  from the models (handlers/middleware query `users`, so the DB must exist before the bot can
  process messages). The models use `JSON().with_variant(JSONB, "postgresql")`, so this works
  on SQLite too. Run a tiny async script:
  ```python
  import asyncio
  from db.session import engine
  from db.models import Base
  async def main():
      async with engine.begin() as conn:
          await conn.run_sync(Base.metadata.create_all)
      await engine.dispose()
  asyncio.run(main())
  ```
  This creates all 8 tables (`users`, `chats`, `downloads`, `banned_links`, `user_settings`,
  `broadcasts`, `broadcast_deliveries`, `music_release_metadata`) on `dev.db`.
- PostgreSQL 16 is installed; start it with `sudo pg_ctlcluster 16 main start`. A `saveinator`
  role/db (password `saveinator`) exists, matching `docker-compose.dev.yml`. Use
  `DATABASE_URL=postgresql+asyncpg://saveinator:saveinator@localhost:5432/saveinator`.

### Live bot / shared token caveat
- The `BOT_TOKEN` secret points at a real bot (`@saveinator_bot`) that already has another
  long-running instance consuming its updates. Running `bot.main` in polling mode therefore
  hits `TelegramConflictError: terminated by other getUpdates request`. Only one poller may
  own a token's update stream at a time, so do NOT leave a dev poller running against a shared
  token. The bot process itself still starts correctly and registers commands with Telegram on
  startup (verify with the Telegram `getMyCommands` API).
- To exercise handler/DB logic without fighting the other poller (or needing a human to DM the
  bot), feed synthetic `Update` objects straight into the dispatcher:
  `dp.feed_update(bot, Update.model_validate({...}, context={"bot": bot}))`. Sends to a
  non-existent chat return `Bad Request: chat not found` (expected); DB writes still commit.

### Tests
- `uv run pytest -q` runs the suite. 12 tests fail on a clean checkout due to stale test
  fakes (e.g. mock `fake_download()` missing the `platform` kwarg the code now passes) and a
  monitoring-assets assertion — these are pre-existing repo issues, not environment problems.
- Note: GitHub Actions CI currently cannot run (the GitHub account is locked for billing), so
  CI status is not a reliable signal for this repo right now.

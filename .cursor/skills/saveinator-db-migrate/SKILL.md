---
name: saveinator-db-migrate
description: >-
  Manages Saveinator PostgreSQL schema via Alembic. Use when migration, Alembic, schema,
  new table, database column, or db/migrations changes.
---

# Saveinator Database Migrations

Schema is managed by **Alembic only**. Go reads the existing PostgreSQL schema — it does **not** run migrations.

## Run migrations

Production / VPS:

```bash
cd /opt/saveinator
docker compose --profile tools run --rm migrate
```

Local dev (`scripts/run-go-dev.sh` does this automatically):

```bash
docker compose -f docker-compose.dev.yml --profile tools build migrate
docker compose -f docker-compose.dev.yml --profile tools run --rm migrate
```

## Add a schema change

1. Edit SQLAlchemy models: `db/models.py`
2. Create Alembic revision: `db/migrations/versions/NNNN_description.py`
3. Run migrate container (above)
4. Update Go code if needed (`go/internal/db/store.go`, etc.)
5. CI: `docker build -f docker/migrate/Dockerfile .` must pass

## Key paths

| Area | Path |
|------|------|
| Models | `db/models.py` |
| Revisions | `db/migrations/versions/` |
| Alembic config | `alembic.ini`, `db/migrations/env.py` |
| Migrate image | `docker/migrate/Dockerfile` |
| Go integration test schema | `go/internal/db/testdata/schema.sql` |

## Go integration tests

DB integration tests use a static schema snapshot. After schema changes, update `go/internal/db/testdata/schema.sql` if integration tests cover new tables/columns.

## Dev vs prod

| Context | Command |
|---------|---------|
| Local dev | `docker-compose.dev.yml` + `--profile tools` |
| Production | `docker-compose.yml` + `--profile tools` |

## Gotchas

- Go `DATABASE_URL` accepts `postgresql+asyncpg://` — normalized to `postgres://` in `config.go`
- Never add `CREATE TABLE` in Go code
- `db/migrations/` history is shared; preserve backward compatibility for existing VPS data volumes

## Related

- Local dev bootstrap: `saveinator-go-dev`
- VPS deploy runs migrate: `saveinator-deploy-vps`

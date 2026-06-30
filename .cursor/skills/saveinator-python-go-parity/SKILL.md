---
name: saveinator-python-go-parity
description: >-
  Keeps Saveinator Python and Go stacks in sync when porting features or fixing bugs.
  Use when changing bot/, go/, link_parser, runtime settings, locales, config, metrics,
  or when the user mentions parity, dual stack, or Python-Go migration.
---

# Saveinator Python ↔ Go Parity

Any user-facing or infra change may need updates in **both** stacks until Python is fully retired.

## Correspondence map

| Concern | Python | Go |
|---------|--------|-----|
| Link parsing | `bot/services/link_parser.py` | `go/internal/linkparser/parser.go` |
| Link tests | `tests/test_link_parser.py` | `go/internal/linkparser/parser_test.go` |
| Config / env | `bot/config.py`, `.env.example` | `go/internal/config/config.go` |
| Runtime settings | `bot/services/runtime_settings.py` | `go/internal/runtime/registry.go` |
| Locales | `locales/{en,ru}.json` | `go/internal/locale/locales/{en,ru}.json` |
| Metrics | `bot/metrics.py` | `go/internal/metrics/metrics.go` |
| Redis helpers | `bot/services/redis_client.py` | `go/internal/redisx/` |
| Handlers | `bot/handlers/` | `go/internal/handler/` |
| Workers / tasks | `workers/tasks.py` (Celery) | `go/internal/worker/` + `go/internal/queue/` |
| DB schema | `db/migrations/` (Alembic) | shared — Go does not run migrations |

## Workflow

Copy this checklist when touching either stack:

```
Parity checklist:
- [ ] Identify affected layers from the table above
- [ ] Update Python side
- [ ] Update Go side (mirror logic, not copy-paste blindly)
- [ ] Sync locale keys (see saveinator-locale-sync skill)
- [ ] Add/update tests in both stacks where applicable
- [ ] Run scripts/check-parity.sh
- [ ] cd go && go test ./...
- [ ] uv run pytest tests/test_<relevant>.py -q
```

## Common drift points

1. **Regex / URL patterns** — must match exactly between `link_parser.py` and `parser.go`
2. **Runtime redis keys** — format `{service}.{kind}` in hash `saveinator:runtime_settings`
3. **Queue task types** — `go/internal/queue/client.go` constants vs Celery task names
4. **Config defaults** — env var names and fallbacks in both `config.py` and `config.go`
5. **Four locale files** — Go embed does not read root `locales/`

## Verification scripts

```bash
# Locale key diff across all 4 JSON files
scripts/check-parity.sh

# Copy missing keys from Python locales → Go locales
scripts/sync-locales.sh
```

## Decision: which stack to change?

| Situation | Action |
|-----------|--------|
| New feature, Go is target | Implement Go first; update Python only if still in prod |
| Bug in prod on Python deploy | Fix Python; port fix to Go |
| User runs `docker-compose.go.yml` | Go is source of truth for that deploy |
| User runs `scripts/deploy.sh` | Python is source of truth for that deploy |

When unsure, ask which stack is deployed before editing one side only.

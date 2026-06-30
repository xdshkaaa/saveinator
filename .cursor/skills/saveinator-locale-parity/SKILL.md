---
name: saveinator-locale-parity
description: >-
  Keeps Saveinator locales and config defaults in sync between root JSON files,
  Go embed, and .env.example. Use when changing locales/, config env vars,
  or running check-parity.sh / sync-locales.sh.
---

# Saveinator Locale & Config Parity

Production is Go-only. Parity checks cover **four locale JSON files** and **config defaults** — not a dual Python/Go app stack.

## Locale files

| Role | Path |
|------|------|
| Canonical EN | `locales/en.json` |
| Canonical RU | `locales/ru.json` |
| Go embed EN | `go/internal/locale/locales/en.json` |
| Go embed RU | `go/internal/locale/locales/ru.json` |

Root `locales/` is canonical. Copy keys to Go embed paths, then rebuild Go (`//go:embed`).

## Workflow

```
Parity checklist:
- [ ] Add keys to locales/en.json and locales/ru.json
- [ ] Copy to go/internal/locale/locales/en.json and ru.json (or scripts/sync-locales.sh)
- [ ] Run scripts/check-parity.sh
- [ ] If new env vars: update .env.example and go/internal/config/config.go defaults
- [ ] cd go && go test ./...
```

## Verification scripts

```bash
# Locale key diff across all 4 JSON files
scripts/check-parity.sh

# Copy missing keys from root locales → Go embed
scripts/sync-locales.sh
```

## Config parity

When adding env vars, keep in sync:

| Source | Path |
|--------|------|
| Example env | `.env.example` |
| Go loader | `go/internal/config/config.go` |

Known mismatch to watch: `PINTEREST_MAX_ITEMS` — `.env.example` may differ from `config.go` default (`10`).

## Related skills

- `saveinator-locale-sync` — step-by-step for new user-facing strings
- `saveinator-go-test` — run tests after locale/config changes

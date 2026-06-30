---
name: saveinator-locale-sync
description: >-
  Syncs Saveinator i18n across four locale JSON files (root canonical + Go embed).
  Use when adding user-facing strings, error messages, onboarding text, or i18n keys.
---

# Saveinator Locale Sync

Four files must stay aligned:

| Role | Path |
|------|------|
| Canonical EN | `locales/en.json` |
| Canonical RU | `locales/ru.json` |
| Go embed EN | `go/internal/locale/locales/en.json` |
| Go embed RU | `go/internal/locale/locales/ru.json` |

Go loads via `//go:embed` in `go/internal/locale/` — **rebuild required** after edits.

## Workflow

```
1. Add key to locales/en.json and locales/ru.json (root — canonical)
2. Copy same keys to go/internal/locale/locales/en.json and ru.json
3. Run scripts/check-parity.sh — must pass
4. Rebuild Go binary if testing locally
```

Or use the sync helper:

```bash
# Copy keys present in root locales but missing in Go embed (both langs)
scripts/sync-locales.sh

# Verify
scripts/check-parity.sh
```

## Placeholder syntax

Go uses `{var}` placeholders:

```go
locale.Get("errors.download_failed", lang, map[string]string{"platform": "tiktok"})
```

Keep placeholder names identical across all four files.

## Nested keys

JSON uses dot-path keys in code (`admin.btn_global`, `errors.rate_limited`). Structure is flat/nested JSON — match existing file style in each file.

## Admin labels (separate from locale JSON)

Runtime setting UI labels live in `go/internal/runtime/registry.go` (`LabelEN` / `LabelRU`). Update when adding admin-visible settings.

## Fallback behavior

| Stack | Missing key |
|-------|-------------|
| Go | en → raw key string |

## Checklist for new strings

- [ ] Key added to `locales/en.json`
- [ ] Key added to `locales/ru.json`
- [ ] Key added to `go/internal/locale/locales/en.json`
- [ ] Key added to `go/internal/locale/locales/ru.json`
- [ ] `scripts/check-parity.sh` passes
- [ ] Go binary rebuilt if running locally

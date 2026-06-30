---
name: saveinator-locale-sync
description: >-
  Syncs Saveinator i18n across four locale JSON files (Python root + Go embed).
  Use when adding user-facing strings, error messages, onboarding text, or i18n keys.
---

# Saveinator Locale Sync

Four files must stay aligned:

| Stack | Path |
|-------|------|
| Python EN | `locales/en.json` |
| Python RU | `locales/ru.json` |
| Go EN | `go/internal/locale/locales/en.json` |
| Go RU | `go/internal/locale/locales/ru.json` |

Go loads via `//go:embed` in `go/internal/locale/` — **rebuild required** after edits.

## Workflow

```
1. Add key to locales/en.json and locales/ru.json (Python — source of truth)
2. Copy same keys to go/internal/locale/locales/en.json and ru.json
3. Run scripts/check-parity.sh — must pass
4. Rebuild Go binary if testing locally
```

Or use the sync helper:

```bash
# Copy keys present in Python but missing in Go (both langs)
scripts/sync-locales.sh

# Verify
scripts/check-parity.sh
```

## Placeholder syntax

Both stacks use `{var}` placeholders:

```go
// Go
locale.Get("errors.download_failed", lang, map[string]string{"platform": "tiktok"})
```

```python
# Python
t("errors.download_failed", platform="tiktok")
```

Keep placeholder names identical across stacks.

## Nested keys

JSON uses dot-path keys in code (`admin.btn_global`, `errors.rate_limited`). Structure is flat/nested JSON — match existing file style in each file.

## Admin labels (separate from locale JSON)

Runtime setting UI labels live in `go/internal/runtime/registry.go` (`LabelEN` / `LabelRU`) and Python `runtime_settings.py`. Update both when adding admin-visible settings.

## Fallback behavior

| Stack | Missing key |
|-------|-------------|
| Go | en → raw key string |
| Python | en → KeyError (tests catch this) |

## Checklist for new strings

- [ ] Key added to `locales/en.json`
- [ ] Key added to `locales/ru.json`
- [ ] Key added to `go/internal/locale/locales/en.json`
- [ ] Key added to `go/internal/locale/locales/ru.json`
- [ ] `scripts/check-parity.sh` passes
- [ ] Go binary rebuilt if running locally

#!/usr/bin/env bash
# Compare locale keys across root locales/ and Go embed i18n files.
set -euo pipefail

ROOT="$(cd "$(dirname "$0")/.." && pwd)"

PY_EN="$ROOT/locales/en.json"
PY_RU="$ROOT/locales/ru.json"
GO_EN="$ROOT/go/internal/locale/locales/en.json"
GO_RU="$ROOT/go/internal/locale/locales/ru.json"

for f in "$PY_EN" "$PY_RU" "$GO_EN" "$GO_RU"; do
  if [[ ! -f "$f" ]]; then
    echo "MISSING: $f"
    exit 1
  fi
done

if ! command -v jq >/dev/null 2>&1; then
  echo "jq is required"
  exit 1
fi

# Flatten nested JSON to dot-separated keys
flatten_keys() {
  jq -r 'paths(scalars) | map(tostring) | join(".")' "$1" | sort -u
}

tmp="$(mktemp -d)"
trap 'rm -rf "$tmp"' EXIT

flatten_keys "$PY_EN" >"$tmp/py_en"
flatten_keys "$PY_RU" >"$tmp/py_ru"
flatten_keys "$GO_EN" >"$tmp/go_en"
flatten_keys "$GO_RU" >"$tmp/go_ru"

errors=0

diff_keys() {
  local label="$1"
  local a="$2"
  local b="$3"
  local only_a only_b
  only_a="$(comm -23 "$a" "$b" || true)"
  only_b="$(comm -13 "$a" "$b" || true)"
  if [[ -n "$only_a" || -n "$only_b" ]]; then
    echo "=== $label ==="
    if [[ -n "$only_a" ]]; then
      echo "Only in first:"
      echo "$only_a" | sed 's/^/  /'
    fi
    if [[ -n "$only_b" ]]; then
      echo "Only in second:"
      echo "$only_b" | sed 's/^/  /'
    fi
    errors=1
  fi
}

diff_keys "en: root vs Go embed" "$tmp/py_en" "$tmp/go_en"
diff_keys "ru: root vs Go embed" "$tmp/py_ru" "$tmp/go_ru"
diff_keys "root en vs ru" "$tmp/py_en" "$tmp/py_ru"
diff_keys "Go en vs ru" "$tmp/go_en" "$tmp/go_ru"

if [[ "$errors" -eq 0 ]]; then
  echo "OK: all locale keys in sync"
  exit 0
fi

echo ""
echo "Fix: add missing keys or run scripts/sync-locales.sh"
exit 1
